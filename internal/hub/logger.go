package hub

import (
	"io"
	"log"
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

// InitLogger creates a zap.Logger based on LogConfig.
// It writes to both console (stderr) and file (if configured).
// It also redirects the standard log package output to zap.
func InitLogger(cfg LogConfig) (*zap.Logger, error) {
	level, err := zapcore.ParseLevel(cfg.Level)
	if err != nil {
		level = zapcore.InfoLevel
	}

	encoderCfg := zap.NewProductionEncoderConfig()
	encoderCfg.TimeKey = "ts"
	encoderCfg.EncodeTime = zapcore.ISO8601TimeEncoder
	encoderCfg.EncodeLevel = zapcore.CapitalLevelEncoder

	// Console core: human-readable
	consoleEncoder := zapcore.NewConsoleEncoder(encoderCfg)
	consoleSyncer := zapcore.AddSync(os.Stderr)
	consoleCore := zapcore.NewCore(consoleEncoder, consoleSyncer, level)

	cores := []zapcore.Core{consoleCore}

	// File core: JSON structured + rotation
	if cfg.File != "" {
		lj := &lumberjack.Logger{
			Filename:   cfg.File,
			MaxSize:    cfg.MaxSizeMB,
			MaxBackups: cfg.MaxBackups,
			MaxAge:     cfg.MaxAgeDays,
			Compress:   cfg.Compress,
		}
		fileSyncer := zapcore.AddSync(lj)
		jsonEncoder := zapcore.NewJSONEncoder(encoderCfg)
		fileCore := zapcore.NewCore(jsonEncoder, fileSyncer, level)
		cores = append(cores, fileCore)
	}

	logger := zap.New(zapcore.NewTee(cores...), zap.AddCaller())

	// Redirect standard log to zap
	stdWriter, err := zap.NewStdLogAt(logger, zapcore.InfoLevel)
	if err == nil {
		log.SetOutput(io.Discard) // suppress default output
		log.SetOutput(stdWriter.Writer())
	}

	return logger, nil
}
