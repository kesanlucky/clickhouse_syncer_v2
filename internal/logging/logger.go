package logging

import (
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

type LogConfig struct {
	Level     string
	Directory string
	Console   bool
}

type Logger struct {
	zapLogger *zap.Logger
	sugar     *zap.SugaredLogger
}

func NewLogger(cfg LogConfig) (*Logger, error) {
	var level zapcore.Level
	if err := level.UnmarshalText([]byte(cfg.Level)); err != nil {
		level = zapcore.InfoLevel
	}

	encoderConfig := zap.NewProductionEncoderConfig()
	encoderConfig.TimeKey = "time"
	encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	encoderConfig.EncodeLevel = zapcore.CapitalLevelEncoder

	var cores []zapcore.Core

	if cfg.Console {
		consoleEncoderConfig := zap.NewDevelopmentEncoderConfig()
		consoleEncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
		consoleCore := zapcore.NewCore(
			zapcore.NewConsoleEncoder(consoleEncoderConfig),
			zapcore.AddSync(os.Stdout),
			level,
		)
		cores = append(cores, consoleCore)
	}

	if cfg.Directory != "" {
		if err := os.MkdirAll(cfg.Directory, 0755); err != nil {
			return nil, fmt.Errorf("failed to create log directory: %w", err)
		}

		appLog := &lumberjack.Logger{
			Filename:   filepath.Join(cfg.Directory, "app.log"),
			MaxSize:    100, // MB
			MaxBackups: 10,
			MaxAge:     30, // days
		}

		errLog := &lumberjack.Logger{
			Filename:   filepath.Join(cfg.Directory, "error.log"),
			MaxSize:    100,
			MaxBackups: 10,
			MaxAge:     30,
		}

		jsonEncoder := zapcore.NewJSONEncoder(encoderConfig)

		appCore := zapcore.NewCore(
			jsonEncoder,
			zapcore.AddSync(appLog),
			level,
		)

		errCore := zapcore.NewCore(
			jsonEncoder,
			zapcore.AddSync(errLog),
			zapcore.ErrorLevel,
		)

		cores = append(cores, appCore, errCore)
	}

	core := zapcore.NewTee(cores...)
	zapLogger := zap.New(core, zap.AddCaller())

	return &Logger{
		zapLogger: zapLogger,
		sugar:     zapLogger.Sugar(),
	}, nil
}

func (l *Logger) Info(msg string, keysAndValues ...any) {
	l.sugar.Infow(msg, keysAndValues...)
}

func (l *Logger) Warn(msg string, keysAndValues ...any) {
	l.sugar.Warnw(msg, keysAndValues...)
}

func (l *Logger) Error(msg string, keysAndValues ...any) {
	l.sugar.Errorw(msg, keysAndValues...)
}

func (l *Logger) Debug(msg string, keysAndValues ...any) {
	l.sugar.Debugw(msg, keysAndValues...)
}

func (l *Logger) Fatal(msg string, keysAndValues ...any) {
	l.sugar.Fatalw(msg, keysAndValues...)
}

func (l *Logger) With(keysAndValues ...any) *Logger {
	sugarWith := l.sugar.With(keysAndValues...)
	return &Logger{
		zapLogger: sugarWith.Desugar(),
		sugar:     sugarWith,
	}
}

func (l *Logger) Sync() error {
	return l.zapLogger.Sync()
}

func (l *Logger) Zap() *zap.Logger {
	return l.zapLogger
}

func GenerateOperationID(op string, date string) string {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	suffix := fmt.Sprintf("%06x", r.Intn(0xffffff))
	return fmt.Sprintf("%s-%s-%s", op, date, suffix)
}
