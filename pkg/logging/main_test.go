package logging

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/stretchr/testify/assert"
	"log/slog"
	"testing"
)

type MockHandler struct {
	slog.Handler
	HandledRecord slog.Record
}

func (h *MockHandler) Handle(_ context.Context, rec slog.Record) error {
	h.HandledRecord = rec
	return nil
}

func TestSanitizingHandler_Handle(t *testing.T) {
	mockHandler := &MockHandler{}
	sanitizingHandler := NewSanitizingLoggerHandler(mockHandler)

	ctx := context.Background()
	record := slog.Record{Message: "Sensitive info: token rwu8j11111111111111888888881pq uuid 12345678-1234-1234-1234-123456789012 login 12345678901 account 123456789012"}

	err := sanitizingHandler.Handle(ctx, record)

	if err != nil {
		t.Errorf("Handle() error = %v", err)
		return
	}

	expected := "Sensitive info: token ****************************** uuid ********-****-****-****-************ login *********** account ************"
	if mockHandler.HandledRecord.Message != expected {
		assert.Equal(t, expected, mockHandler.HandledRecord.Message)
	}
}

func TestSanitizingHandler_CallLogger(t *testing.T) {
	var outputBuffer bytes.Buffer

	logLevel := ParseLogLevel("debug")
	defaultHandler := slog.NewJSONHandler(&outputBuffer, &slog.HandlerOptions{Level: logLevel, AddSource: true})
	logger := slog.New(NewSanitizingLoggerHandler(defaultHandler))

	logger.Debug("Sensitive info: token rwu8j11111111111111888888881pq uuid 12345678-1234-1234-1234-123456789012 login 12345678901 account 123456789012")
	expected := "Sensitive info: token ****************************** uuid ********-****-****-****-************ login *********** account ************"

	var record map[string]interface{}

	err := json.Unmarshal(outputBuffer.Bytes(), &record)
	assert.NoError(t, err)

	assert.Equal(t, expected, record["msg"])
}

func TestSanitizingHandler_Handle_NoSensitiveInfo(t *testing.T) {
	mockHandler := &MockHandler{}
	sanitizingHandler := NewSanitizingLoggerHandler(mockHandler)

	ctx := context.Background()
	record := slog.Record{Message: "No sensitive info here"}

	err := sanitizingHandler.Handle(ctx, record)

	if err != nil {
		t.Errorf("Handle() error = %v", err)
		return
	}

	expected := "No sensitive info here"
	if mockHandler.HandledRecord.Message != expected {
		assert.Equal(t, expected, mockHandler.HandledRecord.Message)
	}
}

func TestParseLogLevel(t *testing.T) {
	tests := []struct {
		name  string
		level string
		want  slog.Level
	}{
		{"Debug level", "debug", slog.LevelDebug},
		{"Info level", "info", slog.LevelInfo},
		{"Warn level", "warn", slog.LevelWarn},
		{"Error level", "error", slog.LevelError},
		{"Default level", "unknown", slog.LevelInfo},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ParseLogLevel(tt.level); got != tt.want {
				t.Errorf("ParseLogLevel() = %v, want %v", got, tt.want)
			}
		})
	}
}
