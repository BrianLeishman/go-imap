package imap

import (
	"log/slog"
	"os"
	"testing"
)

func TestSetLogger_Nil(t *testing.T) {
	t.Parallel()
	// SetLogger(nil) should restore the default logger, not panic.
	SetLogger(nil)
	l := getLogger()
	if l == nil {
		t.Fatal("expected non-nil logger after SetLogger(nil)")
	}
}

func TestSetLogger_SlogBased(t *testing.T) {
	t.Parallel()
	handler := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn})
	logger := slog.New(handler)
	SetLogger(SlogLogger(logger))
	defer SetLogger(nil)

	// Should not panic
	warnLog(-1, "", "test message")
}

func TestSetSlogLogger(t *testing.T) {
	t.Parallel()
	handler := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug})
	logger := slog.New(handler)
	SetSlogLogger(logger)
	defer SetLogger(nil) // restore default

	l := getLogger()
	if l == nil {
		t.Fatal("expected non-nil logger after SetSlogLogger")
	}
}

func TestSlogLogger_Nil(t *testing.T) {
	t.Parallel()
	result := SlogLogger(nil)
	if result != nil {
		t.Fatalf("expected nil, got %v", result)
	}
}

func TestSlogAdapter_AllLevels(t *testing.T) {
	t.Parallel()
	handler := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug})
	logger := SlogLogger(slog.New(handler))

	// These should not panic
	logger.Debug("test debug")
	logger.Info("test info")
	logger.Warn("test warn")
	logger.Error("test error")

	sub := logger.WithAttrs("key", "value")
	if sub == nil {
		t.Fatal("expected non-nil logger from WithAttrs")
	}
	sub.Info("test sub info")
}

func TestDebugLog_VerboseOff(t *testing.T) {
	orig := Verbose
	Verbose = false
	defer func() { Verbose = orig }()

	// Should not panic even when verbose is off
	debugLog(0, "INBOX", "test message")
}

func TestDebugLog_VerboseOn(t *testing.T) {
	orig := Verbose
	Verbose = true
	defer func() { Verbose = orig }()

	// Should not panic
	debugLog(0, "INBOX", "test message")
}

func TestConnectionLogger_NoConn(t *testing.T) {
	t.Parallel()
	l := connectionLogger(-1, "")
	if l == nil {
		t.Fatal("expected non-nil logger")
	}
}

func TestConnectionLogger_WithFolder(t *testing.T) {
	t.Parallel()
	l := connectionLogger(1, "INBOX")
	if l == nil {
		t.Fatal("expected non-nil logger")
	}
}

func TestConnectionLogger_NoFolder(t *testing.T) {
	t.Parallel()
	l := connectionLogger(1, "")
	if l == nil {
		t.Fatal("expected non-nil logger")
	}
}

