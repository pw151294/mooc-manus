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
