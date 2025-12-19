//go:build integration

package imap

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/smtp"
	"os"
	"sync"
	"testing"
	"time"
)

// Integration tests require a running IMAP server.
// Start one with: docker compose up -d
//
// Run tests with: go test -tags=integration -v ./...
//
// Note: These tests modify the global TLSSkipVerify variable and use a mutex
// to prevent race conditions. Do not run with t.Parallel() at the top level.

const (
	testIMAPHost = "localhost"
	testIMAPPort = 3143
	testSMTPHost = "localhost"
	testSMTPPort = 3025
	testUser     = "testuser@localhost"
	testPass     = "testpass"
)

// tlsSkipVerifyMu protects access to the global TLSSkipVerify variable
// to prevent race conditions when tests run concurrently.
var tlsSkipVerifyMu sync.Mutex

func getTestConfig() (host string, imapPort, smtpPort int) {
	host = testIMAPHost
	imapPort = testIMAPPort
	smtpPort = testSMTPPort

	if h := os.Getenv("IMAP_TEST_HOST"); h != "" {
		host = h
	}
	return host, imapPort, smtpPort
}

func waitForServer(host string, port int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", host, port), time.Second)
		if err == nil {
			conn.Close()
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("server %s:%d not ready after %v", host, port, timeout)
}

func sendTestEmail(host string, port int, from, to, subject, body string) error {
	addr := fmt.Sprintf("%s:%d", host, port)
	msg := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\n\r\n%s", from, to, subject, body)
	return smtp.SendMail(addr, nil, from, []string{to}, []byte(msg))
}

func setupTestConnection(t *testing.T) *Dialer {
	t.Helper()

	host, imapPort, smtpPort := getTestConfig()

	// Wait for servers to be ready
	if err := waitForServer(host, imapPort, 30*time.Second); err != nil {
		t.Skipf("IMAP server not available: %v (run: docker compose up -d)", err)
	}
	if err := waitForServer(host, smtpPort, 30*time.Second); err != nil {
		t.Skipf("SMTP server not available: %v (run: docker compose up -d)", err)
	}

	// GreenMail uses non-TLS on port 3143, so we need to connect without TLS
	// For now, let's use the TLS port with skip verify
	tlsSkipVerifyMu.Lock()
	oldSkipVerify := TLSSkipVerify
	TLSSkipVerify = true
	t.Cleanup(func() {
		TLSSkipVerify = oldSkipVerify
		tlsSkipVerifyMu.Unlock()
	})

	// GreenMail creates users on first login attempt
	// Try connecting to the IMAPS port (3993)
	conn, err := New(testUser, testPass, host, 3993)
	if err != nil {
		// If TLS fails, try a plain connection approach
		t.Skipf("Could not connect to IMAP server: %v", err)
	}

	t.Cleanup(func() {
		if conn != nil {
			conn.Close()
		}
	})

	return conn
}

func TestIntegration_GetLastNUIDs(t *testing.T) {
	conn := setupTestConnection(t)

	host, _, smtpPort := getTestConfig()

	// Send some test emails
	numEmails := 15
	for i := 1; i <= numEmails; i++ {
		subject := fmt.Sprintf("Test Email %d", i)
		body := fmt.Sprintf("This is test email number %d", i)
		if err := sendTestEmail(host, smtpPort, "sender@localhost", testUser, subject, body); err != nil {
			t.Fatalf("Failed to send test email %d: %v", i, err)
		}
		// Small delay to ensure ordering
		time.Sleep(100 * time.Millisecond)
	}

	// Give server time to process
	time.Sleep(500 * time.Millisecond)

	// Select INBOX
	if err := conn.SelectFolder("INBOX"); err != nil {
		t.Fatalf("Failed to select INBOX: %v", err)
	}

	// Test GetLastNUIDs
	t.Run("GetLastNUIDs returns correct count", func(t *testing.T) {
		uids, err := conn.GetLastNUIDs(5)
		if err != nil {
			t.Fatalf("GetLastNUIDs failed: %v", err)
		}

		if len(uids) != 5 {
			t.Errorf("Expected 5 UIDs, got %d", len(uids))
		}

		// Verify UIDs are in ascending order
		for i := 1; i < len(uids); i++ {
			if uids[i] <= uids[i-1] {
				t.Errorf("UIDs not in ascending order: %v", uids)
				break
			}
		}
	})

	t.Run("GetLastNUIDs with n=0 returns nil", func(t *testing.T) {
		uids, err := conn.GetLastNUIDs(0)
		if err != nil {
			t.Fatalf("GetLastNUIDs(0) failed: %v", err)
		}
		if uids != nil {
			t.Errorf("Expected nil, got %v", uids)
		}
	})

	t.Run("GetLastNUIDs with negative n returns nil", func(t *testing.T) {
		uids, err := conn.GetLastNUIDs(-5)
		if err != nil {
			t.Fatalf("GetLastNUIDs(-5) failed: %v", err)
		}
		if uids != nil {
			t.Errorf("Expected nil, got %v", uids)
		}
	})

	t.Run("GetLastNUIDs with n greater than total returns all", func(t *testing.T) {
		allUIDs, err := conn.GetUIDs("ALL")
		if err != nil {
			t.Fatalf("GetUIDs(ALL) failed: %v", err)
		}

		lastUIDs, err := conn.GetLastNUIDs(1000)
		if err != nil {
			t.Fatalf("GetLastNUIDs(1000) failed: %v", err)
		}

		if len(lastUIDs) != len(allUIDs) {
			t.Errorf("Expected %d UIDs, got %d", len(allUIDs), len(lastUIDs))
		}
	})

	t.Run("GetLastNUIDs returns highest UIDs", func(t *testing.T) {
		allUIDs, err := conn.GetUIDs("ALL")
		if err != nil {
			t.Fatalf("GetUIDs(ALL) failed: %v", err)
		}

		last5, err := conn.GetLastNUIDs(5)
		if err != nil {
			t.Fatalf("GetLastNUIDs(5) failed: %v", err)
		}

		// The last 5 UIDs from GetLastNUIDs should match the last 5 from GetUIDs("ALL")
		expectedLast5 := allUIDs[len(allUIDs)-5:]
		for i, uid := range last5 {
			if uid != expectedLast5[i] {
				t.Errorf("Mismatch at index %d: got %d, want %d", i, uid, expectedLast5[i])
			}
		}
	})
}

func TestIntegration_GetUIDs_Ranges(t *testing.T) {
	conn := setupTestConnection(t)

	host, _, smtpPort := getTestConfig()

	// Send test emails
	for i := 1; i <= 10; i++ {
		subject := fmt.Sprintf("Range Test %d", i)
		if err := sendTestEmail(host, smtpPort, "sender@localhost", testUser, subject, "body"); err != nil {
			t.Fatalf("Failed to send test email: %v", err)
		}
		time.Sleep(50 * time.Millisecond)
	}

	time.Sleep(500 * time.Millisecond)

	if err := conn.SelectFolder("INBOX"); err != nil {
		t.Fatalf("Failed to select INBOX: %v", err)
	}

	t.Run("GetUIDs ALL returns all messages", func(t *testing.T) {
		uids, err := conn.GetUIDs("ALL")
		if err != nil {
			t.Fatalf("GetUIDs(ALL) failed: %v", err)
		}
		if len(uids) == 0 {
			t.Error("Expected some UIDs, got none")
		}
	})

	t.Run("GetUIDs with * returns single highest UID", func(t *testing.T) {
		// Re-select folder to ensure fresh state
		if err := conn.SelectFolder("INBOX"); err != nil {
			t.Fatalf("Failed to re-select INBOX: %v", err)
		}

		allUIDs, err := conn.GetUIDs("ALL")
		if err != nil {
			t.Fatalf("GetUIDs(ALL) failed: %v", err)
		}
		if len(allUIDs) == 0 {
			t.Skip("No messages in mailbox")
		}

		// The * search should return the highest UID
		// Note: GreenMail sometimes has issues with * searches, so we handle errors gracefully
		starUIDs, err := conn.GetUIDs("*")
		if err != nil {
			t.Skipf("GetUIDs(*) failed (known GreenMail quirk): %v", err)
		}
		if len(starUIDs) != 1 {
			t.Errorf("Expected 1 UID for *, got %d", len(starUIDs))
		}
		if len(starUIDs) > 0 && starUIDs[0] != allUIDs[len(allUIDs)-1] {
			t.Errorf("* should return highest UID: got %d, want %d", starUIDs[0], allUIDs[len(allUIDs)-1])
		}
	})
}

func TestIntegration_Connection(t *testing.T) {
	host, imapPort, _ := getTestConfig()

	if err := waitForServer(host, imapPort, 10*time.Second); err != nil {
		t.Skipf("IMAP server not available: %v", err)
	}

	tlsSkipVerifyMu.Lock()
	oldSkipVerify := TLSSkipVerify
	TLSSkipVerify = true
	defer func() {
		TLSSkipVerify = oldSkipVerify
		tlsSkipVerifyMu.Unlock()
	}()

	t.Run("Connect and authenticate", func(t *testing.T) {
		conn, err := New(testUser, testPass, host, 3993)
		if err != nil {
			t.Fatalf("Failed to connect: %v", err)
		}
		defer conn.Close()

		folders, err := conn.GetFolders()
		if err != nil {
			t.Fatalf("Failed to get folders: %v", err)
		}

		if len(folders) == 0 {
			t.Error("Expected at least one folder")
		}
	})
}

// Ensure tls package is available for TLS connections
var _ = tls.Config{}
