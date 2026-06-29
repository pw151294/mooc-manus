package config

import (
	"log"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

var Cfg *GlobalConfig

type GlobalConfig struct {
	Redis        RedisConfig    `toml:"redis"`
	Database     DatabaseConfig `toml:"database"`
	LoggerConfig LoggerConfig   `toml:"logger"`
	Storage      StorageConfig  `toml:"storage"`
	Skill        SkillConfig    `toml:"skill"`
	Native       NativeConfig   `toml:"native"`
}

type RedisConfig struct {
	Addr         string `toml:"addr"`
	Username     string `toml:"username"`
	Password     string `toml:"password"`
	DB           int    `toml:"db"`
	PoolSize     int    `toml:"pool_size"`
	MinIdleConns int    `toml:"min_idle_conns"`
}

type DatabaseConfig struct {
	User     string `toml:"user"`
	Password string `toml:"password"`
	Database string `toml:"database"`
	Host     string `toml:"host"`
	Port     int    `toml:"port"`
	Sslmode  string `toml:"sslmode"`
}

type StorageConfig struct {
	RootDir string `toml:"root_dir"` // FileStorage 根目录，默认 ./data
}

type SkillConfig struct {
	BaseDir     string            `toml:"base_dir"`      // 容器内基础目录，默认 /data/.beedance
	HostBaseDir string            `toml:"host_base_dir"` // 宿主机基础目录（Docker-in-Docker 场景使用），空值表示非容器化部署，直接使用 BaseDir
	DockerHost  string            `toml:"docker_host"`   // Docker daemon 地址
	DockerImage string            `toml:"docker_image"`  // 默认 Skill 执行镜像
	Env         map[string]string `toml:"env"`           // 注入到 Skill 容器的环境变量（静态配置）
}

// NativeConfig manus 原生内置工具（fileRead / fileEdit / bashExec）配置
// 参见 docs/superpowers/plans/2026-06-29-native-builtin-tools.md 与 .harness/rules/49-native-builtin.md
type NativeConfig struct {
	WorkspaceBaseDir      string   `toml:"workspace_base_dir"`       // fileEdit 写入根目录，默认 ${baseDir}/workspace，与 Skill.BaseDir 对齐
	MaxFileReadBytes      int64    `toml:"max_file_read_bytes"`      // fileRead 单文件读取上限，默认 10 MiB
	SensitivePathDenyList []string `toml:"sensitive_path_deny_list"` // fileRead 敏感路径前缀黑名单
	BashCommandDenyList   []string `toml:"bash_command_deny_list"`   // bashExec 命令正则黑名单（追加到默认基线之上）
	BashTimeoutDefault    int      `toml:"bash_timeout_default"`     // bashExec 默认超时秒数，默认 120
	BashTimeoutMax        int      `toml:"bash_timeout_max"`         // bashExec 超时上限秒数，默认 600
	BashOutputCap         int      `toml:"bash_output_cap"`          // bashExec stdout+stderr 合并截断字节数，默认 32 KiB
	BashConcurrency       int      `toml:"bash_concurrency"`         // bashExec 全局并发上限，默认 4
}

type LoggerConfig struct {
	Level        string `toml:"level"`
	Format       string `toml:"format"`
	Output       string `toml:"output"`
	LogDir       string `toml:"log_dir"`
	LogFile      string `toml:"log_file"`
	MaxSize      int    `toml:"max_size"`
	MaxBackups   int    `toml:"max_backups"`
	MaxAge       int    `toml:"max_age"`
	Compress     bool   `toml:"compress"`
	EnableCaller bool   `toml:"enable_caller"`
	CallerSkip   int    `toml:"caller_skip"`
}

func (c *LoggerConfig) GetLogFilePath() string {
	return filepath.Join(c.LogDir, c.LogFile)
}

func (c *LoggerConfig) EnsureLogDir() error {
	return os.MkdirAll(c.LogDir, 0755)
}

func InitConfig() {
	Cfg = new(GlobalConfig)
	if _, err := toml.DecodeFile("config/config.toml", Cfg); err != nil {
		log.Fatalf("failed to load config from toml: %v", err)
	}
}
