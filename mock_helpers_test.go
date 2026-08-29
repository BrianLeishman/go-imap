package imap

import (
	"bufio"
	"context"
	"strings"
	"testing"
)

// withMockClient starts a mock IMAP server, dials it with valid credentials,
// and returns the connected client plus the mock server. Both are closed
// automatically when the test completes.
func withMockClient(t *testing.T) (*Client, *mockIMAPServer) {
	t.Helper()

	originalVerbose := Verbose
	originalRetry := RetryCount
	originalSkip := TLSSkipVerify
	Verbose = false
	RetryCount = 0
	TLSSkipVerify = true
	t.Cleanup(func() {
		Verbose = originalVerbose
		RetryCount = originalRetry
		TLSSkipVerify = originalSkip
	})

	srv, err := newMockIMAPServer("u", "p")
	if err != nil {
		t.Fatalf("newMockIMAPServer: %v", err)
	}
	t.Cleanup(func() { srv.Close() })

	d, err := Dial(context.Background(), Options{
		Host: srv.GetHost(),
		Port: srv.GetPort(),
		Auth: PasswordAuth{Username: "u", Password: "p"},
	})
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })

	return d, srv
}

// respond is a tiny helper for mock handlers that writes untagged data
// followed by a tagged OK line. Both arguments may be empty.
func respond(w *bufio.Writer, tag, untagged, okMessage string) {
	if untagged != "" {
		if !strings.HasSuffix(untagged, "\r\n") {
			untagged += "\r\n"
		}
		w.WriteString(untagged)
	}
	if okMessage == "" {
		okMessage = "completed"
	}
	w.WriteString(tag + " OK " + okMessage + "\r\n")
}
