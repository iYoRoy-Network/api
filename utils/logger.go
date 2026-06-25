package utils

import (
	"os"
	"path/filepath"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

// LogConfig 日志配置
type LogConfig struct {
	LogLevel string `koanf:"log_level"`
	LogFile  string `koanf:"log_file"`
}

// DefaultLogConfig 返回日志的默认配置
func DefaultLogConfig() LogConfig {
	return LogConfig{
		LogLevel: "info",
		LogFile:  "logs/app.log",
	}
}

// Log 全局 Logger 实例
var Log = zap.NewNop()

// InitLogger 初始化日志，使用模块化配置
func InitLogger(cfg *LogConfig) {
	// 确保日志目录存在
	if cfg.LogFile != "" {
		logDir := filepath.Dir(cfg.LogFile)
		if _, err := os.Stat(logDir); os.IsNotExist(err) {
			_ = os.MkdirAll(logDir, 0755)
		}
	}

	// 设置日志级别
	var level zapcore.Level
	switch cfg.LogLevel {
	case "debug":
		level = zapcore.DebugLevel
	case "info":
		level = zapcore.InfoLevel
	case "warn":
		level = zapcore.WarnLevel
	case "error":
		level = zapcore.ErrorLevel
	default:
		level = zapcore.InfoLevel
	}

	// 编码器配置
	consoleEncoderConfig := zap.NewDevelopmentEncoderConfig()
	consoleEncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	consoleEncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder

	jsonEncoderConfig := zap.NewProductionEncoderConfig()
	jsonEncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	jsonEncoderConfig.EncodeLevel = zapcore.CapitalLevelEncoder

	// 创建多个 Core
	var cores []zapcore.Core

	// 1. 控制台 Core
	consoleCore := zapcore.NewCore(
		zapcore.NewConsoleEncoder(consoleEncoderConfig),
		zapcore.AddSync(os.Stdout),
		level,
	)
	cores = append(cores, consoleCore)

	// 2. 文件 Core (JSON Encoder)
	if cfg.LogFile != "" {
		fileWriter := zapcore.AddSync(&lumberjack.Logger{
			Filename:   cfg.LogFile,
			MaxSize:    10,   // 每个日志文件保存10MB
			MaxBackups: 5,    // 保留5个备份
			MaxAge:     30,   // 保留30天
			Compress:   true, // 是否压缩
		})

		fileCore := zapcore.NewCore(
			zapcore.NewJSONEncoder(jsonEncoderConfig),
			fileWriter,
			level,
		)
		cores = append(cores, fileCore)
	}

	// 使用 NewTee 将多个 Core 合并
	core := zapcore.NewTee(cores...)

	// 创建 Logger，添加调用者信息
	Log = zap.New(core, zap.AddCaller())

	// 替换全局 Logger
	zap.ReplaceGlobals(Log)

	Log.Info("Logger initialized", zap.String("level", cfg.LogLevel))
}
