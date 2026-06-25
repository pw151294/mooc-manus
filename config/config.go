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
	BaseDir     string `toml:"base_dir"`      // 容器内基础目录，默认 /data/.beedance
	HostBaseDir string `toml:"host_base_dir"` // 宿主机基础目录（Docker-in-Docker 场景使用），空值表示非容器化部署，直接使用 BaseDir
	DockerHost  string `toml:"docker_host"`   // Docker daemon 地址
	DockerImage string `toml:"docker_image"`  // 默认 Skill 执行镜像
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
