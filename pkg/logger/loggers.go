package logger

import (
	"mooc-manus/config"
	"os"
	"sync"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

// Logger 日志接口
type Logger interface {
	Debug(msg string, fields ...zap.Field)
	Info(msg string, fields ...zap.Field)
	Warn(msg string, fields ...zap.Field)
	Error(msg string, fields ...zap.Field)
	Panic(msg string, fields ...zap.Field)
	Fatal(msg string, fields ...zap.Field)
	Sync() error
	With(fields ...zap.Field) Logger
}

// ZapLogger 基于zap的日志实现
type ZapLogger struct {
	zap *zap.Logger
}

type LogLevel string

const (
	DebugLevel LogLevel = "debug"
	InfoLevel  LogLevel = "info"
	WarnLevel  LogLevel = "warn"
	ErrorLevel LogLevel = "error"
	PanicLevel LogLevel = "panic"
	FatalLevel LogLevel = "fatal"
)

var (
	globalLogger Logger
	once         sync.Once
)

// NewLogger 创建新的日志实例
func NewLogger(config config.LoggerConfig) (Logger, error) {
	if err := config.EnsureLogDir(); err != nil {
		return nil, err
	}

	// 设置日志级别
	level := getZapLevel(LogLevel(config.Level))

	// 创建编码器配置
	encoderConfig := zapcore.EncoderConfig{
		TimeKey:        "time",
		LevelKey:       "level",
		NameKey:        "logger",
		CallerKey:      "caller",
		MessageKey:     "msg",
		StacktraceKey:  "stacktrace",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.CapitalLevelEncoder,
		EncodeTime:     zapcore.ISO8601TimeEncoder,
		EncodeDuration: zapcore.SecondsDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	}

	// 设置编码器
	var encoder zapcore.Encoder
	if config.Format == "json" {
		encoder = zapcore.NewJSONEncoder(encoderConfig)
	} else {
		encoder = zapcore.NewConsoleEncoder(encoderConfig)
	}

	// 创建多个输出核心
	var cores []zapcore.Core

	// 控制台输出
	if config.Output == "stdout" || config.Output == "both" {
		stdoutCore := zapcore.NewCore(
			encoder,
			zapcore.Lock(os.Stdout),
			level,
		)
		cores = append(cores, stdoutCore)
	}

	// 文件输出
	if config.Output == "file" || config.Output == "both" {
		fileWriter := zapcore.AddSync(&lumberjack.Logger{
			Filename:   config.GetLogFilePath(),
			MaxSize:    config.MaxSize,
			MaxBackups: config.MaxBackups,
			MaxAge:     config.MaxAge,
			Compress:   config.Compress,
		})

		fileCore := zapcore.NewCore(
			encoder,
			fileWriter,
			level,
		)
		cores = append(cores, fileCore)
	}

	// 创建核心
	core := zapcore.NewTee(cores...)

	// 创建选项
	var options []zap.Option
	if config.EnableCaller {
		options = append(options, zap.AddCaller())
		options = append(options, zap.AddCallerSkip(config.CallerSkip))
	}
	options = append(options, zap.AddStacktrace(zap.ErrorLevel))

	zapLogger := zap.New(core, options...)
	return &ZapLogger{
		zap: zapLogger,
	}, nil
}

// InitGlobalLogger 初始化全局日志器
func InitGlobalLogger(config config.LoggerConfig) error {
	var err error
	once.Do(func() {
		globalLogger, err = NewLogger(config)
	})
	return err
}

// GetGlobalLogger 获取全局日志器
func GetGlobalLogger() Logger {
	if globalLogger == nil {
		// 使用默认配置创建临时日志器
		defaultConfig := config.LoggerConfig{}
		logger, _ := NewLogger(defaultConfig)
		return logger
	}
	return globalLogger
}

// Debug 实现Logger接口
func (l *ZapLogger) Debug(msg string, fields ...zap.Field) {
	l.zap.Debug(msg, fields...)
}

func (l *ZapLogger) Info(msg string, fields ...zap.Field) {
	l.zap.Info(msg, fields...)
}

func (l *ZapLogger) Warn(msg string, fields ...zap.Field) {
	l.zap.Warn(msg, fields...)
}

func (l *ZapLogger) Error(msg string, fields ...zap.Field) {
	l.zap.Error(msg, fields...)
}

func (l *ZapLogger) Panic(msg string, fields ...zap.Field) {
	l.zap.Panic(msg, fields...)
}

func (l *ZapLogger) Fatal(msg string, fields ...zap.Field) {
	l.zap.Fatal(msg, fields...)
}

func (l *ZapLogger) Sync() error {
	return l.zap.Sync()
}

func (l *ZapLogger) With(fields ...zap.Field) Logger {
	return &ZapLogger{
		zap: l.zap.With(fields...),
	}
}

// 便捷函数
func Debug(msg string, fields ...zap.Field) {
	GetGlobalLogger().Debug(msg, fields...)
}

func Info(msg string, fields ...zap.Field) {
	GetGlobalLogger().Info(msg, fields...)
}

func Warn(msg string, fields ...zap.Field) {
	GetGlobalLogger().Warn(msg, fields...)
}

func Error(msg string, fields ...zap.Field) {
	GetGlobalLogger().Error(msg, fields...)
}

func Panic(msg string, fields ...zap.Field) {
	GetGlobalLogger().Panic(msg, fields...)
}

func Fatal(msg string, fields ...zap.Field) {
	GetGlobalLogger().Fatal(msg, fields...)
}

func Sync() error {
	return GetGlobalLogger().Sync()
}

// 辅助函数
func getZapLevel(level LogLevel) zapcore.Level {
	switch level {
	case DebugLevel:
		return zap.DebugLevel
	case InfoLevel:
		return zap.InfoLevel
	case WarnLevel:
		return zap.WarnLevel
	case ErrorLevel:
		return zap.ErrorLevel
	case PanicLevel:
		return zap.PanicLevel
	case FatalLevel:
		return zap.FatalLevel
	default:
		return zap.InfoLevel
	}
}

// 便捷字段创建函数
func String(key, value string) zap.Field {
	return zap.String(key, value)
}

func Int(key string, value int) zap.Field {
	return zap.Int(key, value)
}

func Int64(key string, value int64) zap.Field {
	return zap.Int64(key, value)
}

func Float64(key string, value float64) zap.Field {
	return zap.Float64(key, value)
}

func Bool(key string, value bool) zap.Field {
	return zap.Bool(key, value)
}

func Any(key string, value interface{}) zap.Field {
	return zap.Any(key, value)
}

func ErrorField(err error) zap.Field {
	return zap.Error(err)
}
