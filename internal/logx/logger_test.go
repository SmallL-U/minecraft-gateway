package logx

import (
	"io"
	"os"
	"strings"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func TestParseLevel(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    zapcore.Level
		wantErr bool
	}{
		{name: "default", input: "", want: zapcore.InfoLevel},
		{name: "trim", input: "  debug  ", want: zapcore.DebugLevel},
		{name: "case", input: "ErRoR", want: zapcore.ErrorLevel},
		{name: "warning alias", input: "warning", want: zapcore.WarnLevel},
		{name: "invalid", input: "verbose", want: zapcore.InfoLevel, wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := parseLevel(test.input)
			if test.wantErr && err == nil {
				t.Fatal("parseLevel() error = nil, want an error")
			}
			if !test.wantErr && err != nil {
				t.Fatalf("parseLevel() unexpected error: %v", err)
			}
			if got != test.want {
				t.Errorf("parseLevel() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestNewConsoleLoggerIncludesCaller(t *testing.T) {
	readEnd, writeEnd, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() error: %v", err)
	}
	defer readEnd.Close()
	defer writeEnd.Close()

	originalStdout := os.Stdout
	os.Stdout = writeEnd
	t.Cleanup(func() {
		os.Stdout = originalStdout
	})

	testLogger := newConsoleLogger(zap.NewAtomicLevelAt(zapcore.InfoLevel))
	testLogger.Info("caller test")
	if err := writeEnd.Close(); err != nil {
		t.Fatalf("writeEnd.Close() error: %v", err)
	}
	os.Stdout = originalStdout

	output, err := io.ReadAll(readEnd)
	if err != nil {
		t.Fatalf("io.ReadAll() error: %v", err)
	}
	if !strings.Contains(string(output), "logx/logger_test.go:") {
		t.Errorf("log output does not contain caller: %q", output)
	}
}
