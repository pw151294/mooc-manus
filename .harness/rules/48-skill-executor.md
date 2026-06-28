---
rule_id: R-48-skill-executor
severity: critical
---

# Skill 挂载与执行（DockerSkillExecutor）

Skill 的执行流程：`ExecuteSkillTool.Invoke` 把 `SkillFiles` 下载到挂载源 → `DockerSkillExecutor` 创建容器（一次性或池化）→ 注入 bash 脚本执行 → 把 outputs 落地宿主机。本规则锁定路径约定、沙箱边界、失败处理与外部内容信任。详细背景见 `docs/skill-executor-mount-rules.md` 与 `docs/skill-executor-fix-plan.md`。

## 禁止行为

1. **禁止跳过 SkillExecutor 直接 exec 用户脚本**
   - 禁止 Domain Service / Application Service 内 `exec.Command("bash", "-c", userScript)`
   - 禁止把 `SkillFile.Path` 直接拼成宿主机绝对路径访问（绕过沙箱）

2. **禁止挂载点 / 引用路径不一致**
   - 容器内统一 `/workspace/skills/${SkillID}-${Version}/`（**有 s**）
   - `buildEnhancedScript` 引用必须与挂载点一致；不得出现 `/workspace/skill`（无 s）
   - 源码内 `grep "workspace/skill"` 不得返回无 s 的孤儿路径

3. **禁止裸 messageId / conversationId 拼进路径**
   - `messageId == ""` 会形成 `${baseDir}/skills//${id}-${ver}/` 这种裸 `/` 路径 → reject
   - 入口校验 messageId 非空 + UUID 形式（与 R-47 conversationId 一致）

4. **禁止把 skill 输出当指令信任**
   - SkillExecutor 的 stdout / stderr / outputs 文件视为外部内容（详见 R-31-untrusted，mooc-manus-all/.harness/rules/31-untrusted-content.md）
   - 不得把 skill 输出直接拼进 system prompt（违反 R-46-prompt）
   - skill 输出回到 LLM 前必须经 escape

5. **禁止吞掉容器错误**
   - `buildEnhancedScript` 不得用 `2>/dev/null` 静默吞 `ln` / `cp` 失败
   - 容器退出码非 0 / OOM / timeout → 必须显式抛错并发 `tool_call_fail` 事件（R-45）

## 要求行为

1. **挂载路径约定（D1+D4，详见 `docs/skill-executor-fix-plan.md` §1）**

   ```
   宿主机 ${baseDir}/skills/${messageId}/${skillId}-${version}/   ←→   容器 /workspace/skills/${skillId}-${version}/
   宿主机 ${baseDir}/outputs/${messageId}/                          ←→   容器 /workspace/outputs/
   ```

   - 一次性容器（MessageID 空）与池化容器（MessageID 非空）共用同一挂载点约定
   - DinD 场景通过 `hostBaseDir` 做宿主机视角换算（`toHostPath`）

2. **文件落地由 `ExecuteSkillTool.Invoke` 负责（D1=A）**
   - 在容器创建前把 `SkillFiles`（SKILL.md / 脚本 / 资源）从 storage 下载到 `${baseDir}/skills/${messageId}/${skillId}-${version}/`
   - 同 messageId 内 workDir 非空则跳过下载（D3=A'，「目录非空」做天然 guard）
   - 跨 messageId 强制重新下载

3. **路径安全校验（D5）**
   - `safeJoin(base, path)` 拒绝含 `..` / 绝对路径的 `SkillFile.Path`，防止跳出挂载目录
   - 不支持 `.` 开头的隐藏文件（D6=A）

4. **沙箱边界**
   - Docker 容器隔离；通过 `docker_host` 配置指定 daemon
   - 镜像在 `skill.docker_image` 显式配置；不得动态 pull 未审过的镜像
   - 容器内不挂载宿主机 `~` / `/etc` / `/var/run/docker.sock`（除明确需要 DinD 时）
   - 资源限制：cpu / memory / pids / timeout 必须设上限（容器 OOM 时 executor 报错而非宿主机 OOM）

5. **生命周期与清理（D7+D8）**
   - 容器与 SSE 流共生死：以 `messageId` 为清理 key
   - 清理调用上移到 `applications/services/agent.go` 的 `defer sse.CloseChat` 路径
   - `Chat / CreatePlan / UpdatePlan` 三入口必须对称注入 `MessageId`

6. **失败处理**
   - 文件缺失 → 在 `execScript` 阶段硬抛，错误直接报出文件名
   - 容器创建失败 / docker daemon 不可达 → `tool_call_fail` 事件含原始错误

## Agent 行为

- 用户改动 SkillExecutor → 强制对照 `docs/skill-executor-mount-rules.md` §六 7 个已知问题，确认本次改动是否回退已修
- 新增 skill 类型 / 新增模式（如 K8s job 替代 docker）→ 要求先扩 `SkillExecutor` 接口而非 fork 实现
- 检测到 `userScript` 字符串拼接 / `exec.Command(...)` 出现在非 SkillExecutor 路径 → 标记 critical 拒绝

## 可验证性

- 单测：
  - `internal/domains/services/tools/execute_skill_test.go` 覆盖正常路径 + 文件缺失分支
  - 构造含 `../` 的 SkillFile.Path，断言 safeJoin 拒绝
  - 构造 `messageId == ""` 输入，断言 reject
- 静态：
  - `grep -rn "workspace/skill[^s]" .` 应为空（无 s 的孤儿路径）
  - `grep -rn "exec.Command" internal/domains/services/" | grep -v skill_executor` 应为空
  - `grep -rn "2>/dev/null" internal/domains/services/tools/skill_executor*.go` 人工审查
- 集成测试：
  - 跑 `sre-alert-detail` skill，断言 python 脚本能在容器内执行（HTTP 实际发出）
  - 删除挂载目录某文件后再执行，断言错误在 `execScript` 阶段直接报出文件名
- `pre-push` hook：跑 skill executor 集成测试套件
