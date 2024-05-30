package logging

import (
	"context"
	"log/slog"
	"regexp"
)

type SanitizingHandler struct {
	slog.Handler
}

func NewSanitizingLoggerHandler(h slog.Handler) *SanitizingHandler {
	return &SanitizingHandler{h}
}

func (h *SanitizingHandler) Enabled(ctx context.Context, l slog.Level) bool {
	return h.Handler.Enabled(ctx, l)
}

func (h *SanitizingHandler) Handle(ctx context.Context, rec slog.Record) error {
	rec.Message = sanitize(rec.Message)
	return h.Handler.Handle(ctx, rec)
}

func (h *SanitizingHandler) WithAttrs(atts []slog.Attr) slog.Handler {
	return &SanitizingHandler{h.Handler.WithAttrs(atts)}
}

func (h *SanitizingHandler) WithGroup(name string) slog.Handler {
	return &SanitizingHandler{h.Handler.WithGroup(name)}
}

func sanitize(msg string) string {
	tokenRegex := regexp.MustCompile(`[a-z0-9]{30}`)
	uuidRegex := regexp.MustCompile(`\b[0-9a-f]{8}\b-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-\b[0-9a-f]{12}\b`)
	loginRegex := regexp.MustCompile(`\b\d{11}\b`)

	msg = tokenRegex.ReplaceAllString(msg, "**********")
	msg = uuidRegex.ReplaceAllString(msg, "********-****-****-****-************")
	msg = loginRegex.ReplaceAllString(msg, "***********")

	return msg
}

func ParseLogLevel(level string) slog.Level {
	switch level {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	}
	return slog.LevelInfo
}
