package logger

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"time"
)

type Logger struct {
	*slog.Logger
}

type contextKey string

const (
	requestIDKey contextKey = "request_id"
	userIDKey    contextKey = "user_id"
)

func New(level string, format string, output io.Writer) *Logger {
	var logLevel slog.Level
	switch level {
	case "debug":
		logLevel = slog.LevelDebug
	case "info":
		logLevel = slog.LevelInfo
	case "warn":
		logLevel = slog.LevelWarn
	case "error":
		logLevel = slog.LevelError
	default:
		logLevel = slog.LevelInfo
	}

	var handler slog.Handler
	opts := &slog.HandlerOptions{
		Level:     logLevel,
		AddSource: true,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if a.Key == slog.TimeKey {
				return slog.String("timestamp", a.Value.Time().Format(time.RFC3339))
			}
			return a
		},
	}

	if format == "json" {
		handler = slog.NewJSONHandler(output, opts)
	} else {
		handler = slog.NewTextHandler(output, opts)
	}

	return &Logger{slog.New(handler)}
}

func (l *Logger) WithRequestID(ctx context.Context) *Logger {
	if reqID := ctx.Value(requestIDKey); reqID != nil {
		return &Logger{l.Logger.With("request_id", reqID)}
	}
	return l
}

func (l *Logger) WithUserID(ctx context.Context) *Logger {
	if userID := ctx.Value(userIDKey); userID != nil {
		return &Logger{l.Logger.With("user_id", userID)}
	}
	return l
}

func (l *Logger) WithContext(ctx context.Context) *Logger {
	logger := l
	if reqID := ctx.Value(requestIDKey); reqID != nil {
		logger = &Logger{logger.Logger.With("request_id", reqID)}
	}
	if userID := ctx.Value(userIDKey); userID != nil {
		logger = &Logger{logger.Logger.With("user_id", userID)}
	}
	return logger
}

func (l *Logger) Debug(msg string, args ...any) {
	l.Logger.Debug(msg, args...)
}

func (l *Logger) Info(msg string, args ...any) {
	l.Logger.Info(msg, args...)
}

func (l *Logger) Warn(msg string, args ...any) {
	l.Logger.Warn(msg, args...)
}

func (l *Logger) Error(msg string, args ...any) {
	l.Logger.Error(msg, args...)
}

func (l *Logger) ErrorWithContext(ctx context.Context, msg string, err error, args ...any) {
	args = append(args, "error", err.Error())
	l.WithContext(ctx).Logger.Error(msg, args...)
}

func NewContextWithRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, requestIDKey, requestID)
}

func NewContextWithUserID(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, userIDKey, userID)
}

func GetRequestID(ctx context.Context) string {
	if reqID := ctx.Value(requestIDKey); reqID != nil {
		return reqID.(string)
	}
	return ""
}

func GetUserID(ctx context.Context) string {
	if userID := ctx.Value(userIDKey); userID != nil {
		return userID.(string)
	}
	return ""
}

type LogEntry struct {
	Timestamp string         `json:"timestamp"`
	Level     string         `json:"level"`
	Message   string         `json:"message"`
	Fields    map[string]any `json:"fields,omitempty"`
}

func ParseLogLine(line string) (*LogEntry, error) {
	var entry LogEntry
	err := json.Unmarshal([]byte(line), &entry)
	return &entry, err
}

var DefaultLogger = New("info", "text", os.Stdout)