package imap

import (
	"log/slog"
	"os"
	"sync/atomic"
)

// Logger defines the minimal logging interface used by the IMAP client.
//
// Implementations must be safe for concurrent use.
type Logger interface {
	Debug(msg string, args ...any)
	Info(msg string, args ...any)
	Warn(msg string, args ...any)
	Error(msg string, args ...any)
	WithAttrs(args ...any) Logger
}

var globalLogger atomic.Value // stores Logger

func init() {
	globalLogger.Store(defaultLogger())
}

// defaultLogger returns the library's default slog-based logger.
func defaultLogger() Logger {
	handler := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})
	return SlogLogger(slog.New(handler)).WithAttrs("component", "imap/agent")
}

// SetLogger replaces the global logger used by the package. Passing nil
// restores the built-in slog logger.
func SetLogger(logger Logger) {
	if logger == nil {
		globalLogger.Store(defaultLogger())
		return
	}
	globalLogger.Store(logger.WithAttrs("component", "imap/agent"))
}

// SetSlogLogger is a convenience helper for using a *slog.Logger directly.
func SetSlogLogger(logger *slog.Logger) {
	SetLogger(SlogLogger(logger))
}

// SlogLogger adapts a *slog.Logger to the Logger interface.
func SlogLogger(logger *slog.Logger) Logger {
	if logger == nil {
		return nil
	}
	return slogAdapter{logger: logger}
}

type slogAdapter struct {
	logger *slog.Logger
}

func (s slogAdapter) Debug(msg string, args ...any) { s.logger.Debug(msg, args...) }

func (s slogAdapter) Info(msg string, args ...any) { s.logger.Info(msg, args...) }

func (s slogAdapter) Warn(msg string, args ...any) { s.logger.Warn(msg, args...) }

func (s slogAdapter) Error(msg string, args ...any) { s.logger.Error(msg, args...) }

func (s slogAdapter) WithAttrs(args ...any) Logger {
	return slogAdapter{logger: s.logger.With(args...)}
}

// getLogger returns the currently configured logger.
func getLogger() Logger {
	if v := globalLogger.Load(); v != nil {
		if l, ok := v.(Logger); ok {
			return l
		}
	}
	// Fallback for safety if init() was skipped (e.g., in tests).
	l := defaultLogger()
	globalLogger.Store(l)
	return l
}

// connectionLogger adds per-connection context to the configured logger.
func connectionLogger(connNum int, folder string) Logger {
	logger := getLogger()
	// connNum < 0 signals that the caller does not have an active connection
	// context (for example, package-level diagnostics).
	if connNum < 0 && folder == "" {
		return logger
	}

	args := []any{"conn", connNum}
	if folder != "" {
		args = append(args, "mailbox", folder)
	}
	return logger.WithAttrs(args...)
}

// debugLog emits a debug log entry when verbose logging is enabled.
func debugLog(connNum int, folder string, msg string, args ...any) {
	if !Verbose {
		return
	}
	connectionLogger(connNum, folder).Debug(msg, args...)
}

func warnLog(connNum int, folder string, msg string, args ...any) {
	connectionLogger(connNum, folder).Warn(msg, args...)
}

func errorLog(connNum int, folder string, msg string, args ...any) {
	connectionLogger(connNum, folder).Error(msg, args...)
}
