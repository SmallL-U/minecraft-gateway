package logx

import (
	"fmt"
	"os"
	"strings"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

const defaultLogLevel = "info"

var (
	atomicLevel = zap.NewAtomicLevelAt(zapcore.InfoLevel)
	logger      *zap.SugaredLogger
)

func init() {
	logger = newConsoleLogger(atomicLevel)
}

func newConsoleLogger(level zap.AtomicLevel) *zap.SugaredLogger {
	encoderConfig := zapcore.EncoderConfig{
		TimeKey:        "time",
		LevelKey:       "level",
		NameKey:        "logger",
		CallerKey:      "caller",
		MessageKey:     "msg",
		StacktraceKey:  "stacktrace",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.CapitalColorLevelEncoder,
		EncodeTime:     zapcore.TimeEncoderOfLayout("2006-01-02 15:04:05"),
		EncodeDuration: zapcore.StringDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	}

	core := zapcore.NewCore(
		zapcore.NewConsoleEncoder(encoderConfig),
		zapcore.AddSync(os.Stdout),
		level,
	)

	return zap.New(core).Sugar()
}

func SetLevel(level string) error {
	parsedLevel, err := parseLevel(level)
	if err != nil {
		return err
	}
	atomicLevel.SetLevel(parsedLevel)
	return nil
}

func parseLevel(level string) (zapcore.Level, error) {
	normalizedLevel := strings.TrimSpace(strings.ToLower(level))
	if normalizedLevel == "" {
		normalizedLevel = defaultLogLevel
	}
	if normalizedLevel == "warning" {
		normalizedLevel = "warn"
	}

	switch normalizedLevel {
	case "debug":
		return zapcore.DebugLevel, nil
	case "info":
		return zapcore.InfoLevel, nil
	case "warn":
		return zapcore.WarnLevel, nil
	case "error":
		return zapcore.ErrorLevel, nil
	default:
		return zapcore.InfoLevel, fmt.Errorf("invalid log level %q", level)
	}
}

func GetLogger() *zap.SugaredLogger {
	return logger
}
