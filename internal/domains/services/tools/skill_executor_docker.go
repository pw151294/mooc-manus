package tools

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"

	"mooc-manus/pkg/logger"

	"go.uber.org/zap"
)

// DockerSkillExecutor 基于 Docker 容器的 Skill 执行器
type DockerSkillExecutor struct {
	baseDir     string
	hostBaseDir string
	dockerHost  string
	dockerImage string
	staticEnv   map[string]string // 静态环境变量（从配置注入到所有容器）

	dockerClient *client.Client
	clientOnce   sync.Once
	clientErr    error

	containerPool sync.Map // key=poolKey, value=*containerContext
	fileCache     sync.Map // key=poolKey, value=string(workDir)
}

type containerContext struct {
	containerID string
	workDir     string
	createdAt   time.Time
}

const (
	skillExecutionTimeout = 30 * time.Second
	dockerStopTimeout     = 10 // 秒
	containerLabelKey     = "created_by"
	containerLabelValue   = "mooc-manus-skill-executor"
)

// NewDockerSkillExecutor 创建 Docker 执行器实例
// baseDir / hostBaseDir 支持相对路径，构造时统一规范为绝对路径
// 否则 Docker bind mount 会因 Source 非绝对路径被 daemon 拒绝
func NewDockerSkillExecutor(baseDir, hostBaseDir, dockerHost, dockerImage string, staticEnv map[string]string) SkillExecutor {
	if abs, err := filepath.Abs(baseDir); err == nil {
		baseDir = abs
	}
	if hostBaseDir != "" {
		if abs, err := filepath.Abs(hostBaseDir); err == nil {
			hostBaseDir = abs
		}
	}

	logger.Info("[skill-exec] DockerSkillExecutor initialized",
		zap.String("base_dir", baseDir),
		zap.String("host_base_dir", hostBaseDir),
		zap.String("docker_host", dockerHost),
		zap.String("docker_image", dockerImage),
		zap.Strings("static_env_keys", envKeys(staticEnv)),
	)

	return &DockerSkillExecutor{
		baseDir:     baseDir,
		hostBaseDir: hostBaseDir,
		dockerHost:  dockerHost,
		dockerImage: dockerImage,
		staticEnv:   staticEnv,
	}
}

// getDockerClient 懒加载 Docker 客户端（线程安全）
func (e *DockerSkillExecutor) getDockerClient() (*client.Client, error) {
	e.clientOnce.Do(func() {
		opts := []client.Opt{client.WithAPIVersionNegotiation()}
		if e.dockerHost != "" {
			opts = append(opts, client.WithHost(e.dockerHost))
		}
		cli, err := client.NewClientWithOpts(opts...)
		if err != nil {
			e.clientErr = wrapErr("getDockerClient", err)
			return
		}
		e.dockerClient = cli
	})
	if e.clientErr != nil {
		return nil, e.clientErr
	}
	return e.dockerClient, nil
}

// buildPoolKey 构建容器池 Key：messageID:skillID:version
func (e *DockerSkillExecutor) buildPoolKey(ctx SkillExecutionContext) string {
	return fmt.Sprintf("%s:%s:%s", ctx.MessageID, ctx.SkillID, ctx.Version)
}

// getSkillWorkDir 获取 Skill 工作目录：${baseDir}/skills/${messageID}/${skillID}-${version}
func (e *DockerSkillExecutor) getSkillWorkDir(ctx SkillExecutionContext) string {
	skillDirName := fmt.Sprintf("%s-%s", ctx.SkillID, ctx.Version)
	return filepath.Join(e.baseDir, "skills", ctx.MessageID, skillDirName)
}

// getSkillsMessageDir 获取消息级别 skills 目录（容器挂载点）
func (e *DockerSkillExecutor) getSkillsMessageDir(ctx SkillExecutionContext) string {
	return filepath.Join(e.baseDir, "skills", ctx.MessageID)
}

// getOutputsDir 获取输出目录：${baseDir}/outputs/${messageID}
func (e *DockerSkillExecutor) getOutputsDir(ctx SkillExecutionContext) string {
	return filepath.Join(e.baseDir, "outputs", ctx.MessageID)
}

// toHostPath 将容器内路径转换为宿主机路径（Docker-in-Docker 场景）
// hostBaseDir 为空表示非 DinD 部署，直接使用容器内路径
func (e *DockerSkillExecutor) toHostPath(containerPath string) string {
	if e.hostBaseDir == "" {
		return containerPath
	}
	relPath, err := filepath.Rel(e.baseDir, containerPath)
	if err != nil {
		return containerPath
	}
	return filepath.Join(e.hostBaseDir, relPath)
}

// scanFiles 递归扫描目录，返回所有文件相对路径集合
func (e *DockerSkillExecutor) scanFiles(dir string) map[string]bool {
	files := make(map[string]bool)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return files
	}
	_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if rel, e2 := filepath.Rel(dir, path); e2 == nil {
			files[rel] = true
		}
		return nil
	})
	return files
}

// buildEnvList 将 staticEnv 构造为 "KEY=VALUE" 列表（供容器 Env 字段使用）
// 按 key 字典序排序，确保容器配置稳定（便于排查和容器复用判断）
func (e *DockerSkillExecutor) buildEnvList() []string {
	if len(e.staticEnv) == 0 {
		return nil
	}
	keys := make([]string, 0, len(e.staticEnv))
	for k := range e.staticEnv {
		keys = append(keys, k)
	}
	// 稳定排序，避免相同 env 产生不同容器 hash
	sort.Strings(keys)
	envList := make([]string, 0, len(e.staticEnv))
	for _, k := range keys {
		envList = append(envList, fmt.Sprintf("%s=%s", k, e.staticEnv[k]))
	}
	return envList
}

// envKeys 返回 env map 的 key 列表（用于日志脱敏，避免打印敏感 token 值）
func envKeys(m map[string]string) []string {
	if len(m) == 0 {
		return nil
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// SkillFilesMissingExitCode 标识"挂载目录里没有 skill 文件"的特殊退出码（T4）
// Go 侧 execScript 完成后判断 exitCode == 70，包装成可读的错误回灌给 LLM
const SkillFilesMissingExitCode = 70

// buildEnhancedScript 拼接容器内执行脚本：检查 skill 文件存在性 → 软链到 outputs → export → 执行业务命令
// 使用 set -e + 显式存在性检查 + exit 70，确保 skill 文件缺失时在最近位置硬失败
func (e *DockerSkillExecutor) buildEnhancedScript(ctx SkillExecutionContext, bashCommand string) string {
	skillDirName := fmt.Sprintf("%s-%s", ctx.SkillID, ctx.Version)
	skillDir := fmt.Sprintf("/workspace/skills/%s", skillDirName)
	finalScript := fmt.Sprintf(`set -e
cd /workspace/outputs
if ! ls %s/*.py >/dev/null 2>&1 && ! ls %s/SKILL.md >/dev/null 2>&1; then
  echo "[skill-exec] FATAL: no skill files found in %s" >&2
  exit %d
fi
ln -sf %s/* /workspace/outputs/
export SKILL_DIR=%s
%s`,
		skillDir, skillDir, skillDir, SkillFilesMissingExitCode,
		skillDir, skillDir, bashCommand)

	logger.Info("[skill-exec] buildEnhancedScript",
		zap.String("skill_dir_in_container", skillDir),
		zap.String("original_bash_command", bashCommand),
		zap.String("enhanced_script", finalScript),
	)

	return finalScript
}

// wrapErr 错误包装
func wrapErr(operation string, err error) error {
	return fmt.Errorf("DockerSkillExecutor.%s failed: %w", operation, err)
}

// Execute 执行入口：根据 MessageID 选择执行模式
func (e *DockerSkillExecutor) Execute(ctx SkillExecutionContext, bashCommand string) ([]SkillExecutionResult, error) {
	logger.Info("[skill-exec] Execute called",
		zap.String("skill_id", ctx.SkillID),
		zap.String("version", ctx.Version),
		zap.String("message_id", ctx.MessageID),
		zap.Bool("use_container_pool", ctx.MessageID != ""),
		zap.String("bash_command", bashCommand),
	)

	if ctx.MessageID == "" {
		return e.executeWithDisposableContainer(ctx, bashCommand)
	}
	return e.executeWithContainerPool(ctx, bashCommand)
}

// createContainer 创建并启动容器，挂载 skillWorkDir 和 outputsDir
// 返回容器 ID；调用方负责最终清理容器
// 挂载到容器内 /workspace/skills/${SkillID}-${Version}/，与 buildEnhancedScript 引用路径对齐
func (e *DockerSkillExecutor) createContainer(
	dockerCtx context.Context,
	cli *client.Client,
	ctx SkillExecutionContext,
	skillWorkDir, outputsDir string,
) (string, error) {
	// 确保宿主机挂载目录存在
	if err := os.MkdirAll(skillWorkDir, 0755); err != nil {
		return "", wrapErr("createContainer.MkdirAll(skillWorkDir)", err)
	}
	if err := os.MkdirAll(outputsDir, 0755); err != nil {
		return "", wrapErr("createContainer.MkdirAll(outputsDir)", err)
	}

	hostSkillDir := e.toHostPath(skillWorkDir)
	hostOutputsDir := e.toHostPath(outputsDir)
	// 容器内挂载点：/workspace/skills/${SkillID}-${Version}/
	containerSkillMount := fmt.Sprintf("/workspace/skills/%s-%s", ctx.SkillID, ctx.Version)

	logger.Info("[skill-exec] createContainer paths (disposable mode)",
		zap.String("skill_work_dir", skillWorkDir),
		zap.String("outputs_dir", outputsDir),
		zap.String("host_skill_dir_mount_source", hostSkillDir),
		zap.String("host_outputs_dir_mount_source", hostOutputsDir),
		zap.String("container_mount_target_skill", containerSkillMount),
		zap.String("container_mount_target_outputs", "/workspace/outputs"),
		zap.Strings("env_keys", envKeys(e.staticEnv)),
		zap.String("docker_image", e.dockerImage),
	)

	cfg := &container.Config{
		Image: e.dockerImage,
		// 容器保持运行，等待 exec 注入命令
		Cmd:        []string{"tail", "-f", "/dev/null"},
		WorkingDir: "/workspace/outputs",
		Env:        e.buildEnvList(), // 注入静态环境变量
		Labels: map[string]string{
			containerLabelKey: containerLabelValue,
		},
	}
	hostCfg := &container.HostConfig{
		Mounts: []mount.Mount{
			{
				Type:   mount.TypeBind,
				Source: hostSkillDir,
				Target: containerSkillMount,
			},
			{
				Type:   mount.TypeBind,
				Source: hostOutputsDir,
				Target: "/workspace/outputs",
			},
		},
		// 资源限制：防止失控脚本耗尽宿主资源
		Resources: container.Resources{
			Memory:   512 * 1024 * 1024, // 512 MiB
			NanoCPUs: 1_000_000_000,     // 1 CPU
		},
	}

	resp, err := cli.ContainerCreate(dockerCtx, cfg, hostCfg, nil, nil, "")
	if err != nil {
		return "", wrapErr("createContainer.ContainerCreate", err)
	}

	logger.Info("[skill-exec] disposable container created", zap.String("container_id", resp.ID))

	if err := cli.ContainerStart(dockerCtx, resp.ID, container.StartOptions{}); err != nil {
		// 启动失败，尽力删除已创建的容器
		_ = cli.ContainerRemove(dockerCtx, resp.ID, container.RemoveOptions{Force: true})
		return "", wrapErr("createContainer.ContainerStart", err)
	}

	return resp.ID, nil
}

// execScript 在运行中的容器内执行 bash 命令，返回 stdout、stderr 和退出码
func (e *DockerSkillExecutor) execScript(
	dockerCtx context.Context,
	cli *client.Client,
	containerID, bashCommand string,
) (stdout, stderr string, exitCode int, err error) {
	logger.Info("[skill-exec] execScript begin",
		zap.String("container_id", containerID),
		zap.String("bash_command", bashCommand),
	)

	execCfg := container.ExecOptions{
		AttachStdout: true,
		AttachStderr: true,
		Cmd:          []string{"/bin/bash", "-c", bashCommand},
		WorkingDir:   "/workspace/outputs",
	}

	execResp, err := cli.ContainerExecCreate(dockerCtx, containerID, execCfg)
	if err != nil {
		return "", "", -1, wrapErr("execScript.ExecCreate", err)
	}

	attachResp, err := cli.ContainerExecAttach(dockerCtx, execResp.ID, container.ExecStartOptions{})
	if err != nil {
		return "", "", -1, wrapErr("execScript.ExecAttach", err)
	}
	defer attachResp.Close()

	var stdoutBuf, stderrBuf bytes.Buffer
	// stdcopy.StdCopy 从 Docker 多路复用流中分离 stdout 和 stderr
	if _, err = stdcopy.StdCopy(&stdoutBuf, &stderrBuf, attachResp.Reader); err != nil {
		return "", "", -1, wrapErr("execScript.StdCopy", err)
	}

	// 获取退出码（需等待流读取完成后再 inspect）
	inspect, err := cli.ContainerExecInspect(dockerCtx, execResp.ID)
	if err != nil {
		return stdoutBuf.String(), stderrBuf.String(), -1, wrapErr("execScript.ExecInspect", err)
	}

	logger.Info("[skill-exec] execScript completed",
		zap.Int("exit_code", inspect.ExitCode),
		zap.Int("stdout_bytes", stdoutBuf.Len()),
		zap.Int("stderr_bytes", stderrBuf.Len()),
	)

	return stdoutBuf.String(), stderrBuf.String(), inspect.ExitCode, nil
}

// stopAndRemoveContainer 停止并删除容器（force remove，忽略停止错误）
func (e *DockerSkillExecutor) stopAndRemoveContainer(dockerCtx context.Context, cli *client.Client, containerID string) {
	timeout := dockerStopTimeout
	_ = cli.ContainerStop(dockerCtx, containerID, container.StopOptions{Timeout: &timeout})
	_ = cli.ContainerRemove(dockerCtx, containerID, container.RemoveOptions{Force: true})
}

// executeWithDisposableContainer 一次性容器模式
// 流程：准备目录 → 创建容器 → exec 脚本 → 收集产物 → 删除容器 → 返回结果
func (e *DockerSkillExecutor) executeWithDisposableContainer(ctx SkillExecutionContext, bashCommand string) ([]SkillExecutionResult, error) {
	cli, err := e.getDockerClient()
	if err != nil {
		return nil, err
	}

	skillWorkDir := e.getSkillWorkDir(ctx)
	outputsDir := e.getOutputsDir(ctx)

	// 执行前快照 outputs 目录文件集合（用于计算新增产物）
	beforeFiles := e.scanFiles(outputsDir)

	dockerCtx, cancel := context.WithTimeout(context.Background(), skillExecutionTimeout)
	defer cancel()

	containerID, err := e.createContainer(dockerCtx, cli, ctx, skillWorkDir, outputsDir)
	if err != nil {
		return nil, err
	}
	// 无论成功与否，最终清理容器
	defer e.stopAndRemoveContainer(context.Background(), cli, containerID)

	enhancedScript := e.buildEnhancedScript(ctx, bashCommand)
	stdout, stderr, exitCode, err := e.execScript(dockerCtx, cli, containerID, enhancedScript)
	if err != nil {
		return nil, err
	}

	status := "completed"
	if exitCode != 0 {
		status = "failed"
	}
	if exitCode == SkillFilesMissingExitCode {
		stderr = fmt.Sprintf("Skill 文件缺失（exit %d）：容器挂载目录里没有可执行的 skill 文件。"+
			"通常原因：宿主机 skill 工作目录未准备好，或挂载路径不一致。\n原始 stderr: %s",
			SkillFilesMissingExitCode, stderr)
	}

	// 计算新增产物文件（执行后快照 - 执行前快照）
	afterFiles := e.scanFiles(outputsDir)
	var outputFiles []string
	for rel := range afterFiles {
		if !beforeFiles[rel] {
			hostPath := e.toHostPath(filepath.Join(outputsDir, rel))
			outputFiles = append(outputFiles, hostPath)
		}
	}

	result := SkillExecutionResult{
		Stdout:      stdout,
		Stderr:      stderr,
		Status:      status,
		OutputFiles: outputFiles,
	}
	return []SkillExecutionResult{result}, nil
}

// createPooledContainer 为 messageID 创建并缓存长生命周期容器
// 挂载 skills/messageID 目录和 outputs/messageID 目录，容器在 CleanupMessage 时销毁
func (e *DockerSkillExecutor) createPooledContainer(
	dockerCtx context.Context,
	cli *client.Client,
	ctx SkillExecutionContext,
) (string, error) {
	skillsMessageDir := e.getSkillsMessageDir(ctx)
	outputsDir := e.getOutputsDir(ctx)

	if err := os.MkdirAll(skillsMessageDir, 0755); err != nil {
		return "", wrapErr("createPooledContainer.MkdirAll(skillsMessageDir)", err)
	}
	if err := os.MkdirAll(outputsDir, 0755); err != nil {
		return "", wrapErr("createPooledContainer.MkdirAll(outputsDir)", err)
	}

	hostSkillsDir := e.toHostPath(skillsMessageDir)
	hostOutputsDir := e.toHostPath(outputsDir)

	logger.Info("[skill-exec] createPooledContainer paths (pool mode)",
		zap.String("skills_message_dir", skillsMessageDir),
		zap.String("outputs_dir", outputsDir),
		zap.String("host_skills_dir_mount_source", hostSkillsDir),
		zap.String("host_outputs_dir_mount_source", hostOutputsDir),
		zap.String("container_mount_target_skills", "/workspace/skills"),
		zap.String("container_mount_target_outputs", "/workspace/outputs"),
		zap.Strings("env_keys", envKeys(e.staticEnv)),
		zap.String("docker_image", e.dockerImage),
		zap.String("message_id", ctx.MessageID),
	)

	cfg := &container.Config{
		Image:      e.dockerImage,
		Cmd:        []string{"tail", "-f", "/dev/null"},
		WorkingDir: "/workspace/outputs",
		Env:        e.buildEnvList(), // 注入静态环境变量
		Labels: map[string]string{
			containerLabelKey: containerLabelValue,
			"mooc_message_id": ctx.MessageID,
		},
	}
	hostCfg := &container.HostConfig{
		Mounts: []mount.Mount{
			{
				Type:   mount.TypeBind,
				Source: hostSkillsDir,
				Target: "/workspace/skills",
			},
			{
				Type:   mount.TypeBind,
				Source: hostOutputsDir,
				Target: "/workspace/outputs",
			},
		},
		Resources: container.Resources{
			Memory:   512 * 1024 * 1024,
			NanoCPUs: 1_000_000_000,
		},
	}

	resp, err := cli.ContainerCreate(dockerCtx, cfg, hostCfg, nil, nil, "")
	if err != nil {
		return "", wrapErr("createPooledContainer.ContainerCreate", err)
	}

	logger.Info("[skill-exec] pooled container created", zap.String("container_id", resp.ID))

	if err := cli.ContainerStart(dockerCtx, resp.ID, container.StartOptions{}); err != nil {
		_ = cli.ContainerRemove(dockerCtx, resp.ID, container.RemoveOptions{Force: true})
		return "", wrapErr("createPooledContainer.ContainerStart", err)
	}

	return resp.ID, nil
}

// executeWithContainerPool 容器池复用模式
// 同一 messageID 复用同一个容器（挂载整个 skills/messageID），脚本可跨多次调用共享文件
func (e *DockerSkillExecutor) executeWithContainerPool(ctx SkillExecutionContext, bashCommand string) ([]SkillExecutionResult, error) {
	cli, err := e.getDockerClient()
	if err != nil {
		return nil, err
	}

	poolKey := ctx.MessageID
	outputsDir := e.getOutputsDir(ctx)
	beforeFiles := e.scanFiles(outputsDir)

	// 取容器 ID（先从池中查，没有则新建）
	var containerID string
	if val, ok := e.containerPool.Load(poolKey); ok {
		containerID = val.(*containerContext).containerID
	} else {
		createCtx, createCancel := context.WithTimeout(context.Background(), skillExecutionTimeout)
		id, createErr := e.createPooledContainer(createCtx, cli, ctx)
		createCancel()
		if createErr != nil {
			return nil, createErr
		}
		e.containerPool.Store(poolKey, &containerContext{
			containerID: id,
			workDir:     e.getSkillsMessageDir(ctx),
			createdAt:   time.Now(),
		})
		containerID = id
	}

	dockerCtx, cancel := context.WithTimeout(context.Background(), skillExecutionTimeout)
	defer cancel()

	enhancedScript := e.buildEnhancedScript(ctx, bashCommand)
	stdout, stderr, exitCode, execErr := e.execScript(dockerCtx, cli, containerID, enhancedScript)
	if execErr != nil {
		return nil, execErr
	}

	status := "completed"
	if exitCode != 0 {
		status = "failed"
	}
	if exitCode == SkillFilesMissingExitCode {
		stderr = fmt.Sprintf("Skill 文件缺失（exit %d）：容器挂载目录里没有可执行的 skill 文件。"+
			"通常原因：宿主机 skill 工作目录未准备好，或挂载路径不一致。\n原始 stderr: %s",
			SkillFilesMissingExitCode, stderr)
	}

	afterFiles := e.scanFiles(outputsDir)
	var outputFiles []string
	for rel := range afterFiles {
		if !beforeFiles[rel] {
			hostPath := e.toHostPath(filepath.Join(outputsDir, rel))
			outputFiles = append(outputFiles, hostPath)
		}
	}

	return []SkillExecutionResult{{
		Stdout:      stdout,
		Stderr:      stderr,
		Status:      status,
		OutputFiles: outputFiles,
	}}, nil
}

// CleanupMessage 清理指定 messageID 关联的容器与 skills 目录（保留 outputs）
// 在对话/消息生命周期结束时调用
func (e *DockerSkillExecutor) CleanupMessage(messageID string) error {
	if messageID == "" {
		return nil
	}

	skillsMessageDir := filepath.Join(e.baseDir, "skills", messageID)

	logger.Info("[skill-exec] CleanupMessage",
		zap.String("message_id", messageID),
		zap.String("skills_message_dir_to_remove", skillsMessageDir),
	)

	// 停止并删除容器池中对应的容器
	if val, ok := e.containerPool.LoadAndDelete(messageID); ok {
		cc := val.(*containerContext)
		logger.Info("[skill-exec] CleanupMessage removing pooled container",
			zap.String("container_id", cc.containerID),
		)
		cli, err := e.getDockerClient()
		if err == nil {
			e.stopAndRemoveContainer(context.Background(), cli, cc.containerID)
		}
	}

	// 删除 skills/messageID 目录（outputs 目录保留，供调用方使用产物）
	if err := os.RemoveAll(skillsMessageDir); err != nil && !os.IsNotExist(err) {
		return wrapErr("CleanupMessage.RemoveAll(skillsMessageDir)", err)
	}

	return nil
}

// 编译期接口契约校验
var _ SkillExecutor = (*DockerSkillExecutor)(nil)
