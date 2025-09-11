package imap

import (
	"bufio"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// mockIMAPServer creates a simple IMAP server for testing
type mockIMAPServer struct {
	listener       net.Listener
	address        string
	authAttempts   int32
	validUser      string
	validPass      string
	failAuth       bool
	failConnection bool
	responses      map[string]string
	tlsConfig      *tls.Config
}

func newMockIMAPServer(validUser, validPass string) (*mockIMAPServer, error) {
	// Generate a certificate for testing
	cert, err := generateSelfSignedCertificate()
	if err != nil {
		return nil, fmt.Errorf("failed to generate certificate: %v", err)
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
	}

	listener, err := tls.Listen("tcp", "127.0.0.1:0", tlsConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create TLS listener: %v", err)
	}

	server := &mockIMAPServer{
		listener:  listener,
		address:   listener.Addr().String(),
		validUser: validUser,
		validPass: validPass,
		responses: make(map[string]string),
		tlsConfig: tlsConfig,
	}

	go server.serve()
	return server, nil
}

func (s *mockIMAPServer) serve() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			return
		}
		go s.handleConnection(conn)
	}
}

func (s *mockIMAPServer) handleConnection(conn net.Conn) {
	defer conn.Close()

	if s.failConnection {
		// Simulate connection failure
		return
	}

	reader := bufio.NewReader(conn)
	writer := bufio.NewWriter(conn)

	// Send greeting
	writer.WriteString("* OK IMAP4rev1 Mock Server Ready\r\n")
	writer.Flush()

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}

		line = strings.TrimSpace(line)
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}

		tag := parts[0]
		command := strings.ToUpper(parts[1])

		switch command {
		case "LOGIN":
			atomic.AddInt32(&s.authAttempts, 1)
			if s.failAuth {
				writer.WriteString(fmt.Sprintf("%s NO LOGIN failed\r\n", tag))
			} else if len(parts) >= 4 {
				// Extract username and password (removing quotes)
				username := strings.Trim(parts[2], `"`)
				password := strings.Trim(parts[3], `"`)

				if username == s.validUser && password == s.validPass {
					writer.WriteString(fmt.Sprintf("%s OK LOGIN completed\r\n", tag))
				} else {
					writer.WriteString(fmt.Sprintf("%s NO [AUTHENTICATIONFAILED] Authentication failed\r\n", tag))
				}
			} else {
				writer.WriteString(fmt.Sprintf("%s BAD Invalid LOGIN command\r\n", tag))
			}

		case "AUTHENTICATE":
			atomic.AddInt32(&s.authAttempts, 1)
			if s.failAuth {
				writer.WriteString(fmt.Sprintf("%s NO AUTHENTICATE failed\r\n", tag))
			} else {
				// Simplified XOAUTH2 handling
				writer.WriteString(fmt.Sprintf("%s OK AUTHENTICATE completed\r\n", tag))
			}

		case "CAPABILITY":
			writer.WriteString("* CAPABILITY IMAP4rev1 LOGIN AUTHENTICATE\r\n")
			writer.WriteString(fmt.Sprintf("%s OK CAPABILITY completed\r\n", tag))

		case "LOGOUT":
			writer.WriteString("* BYE IMAP4rev1 Server logging out\r\n")
			writer.WriteString(fmt.Sprintf("%s OK LOGOUT completed\r\n", tag))
			return

		default:
			writer.WriteString(fmt.Sprintf("%s OK %s completed\r\n", tag, command))
		}

		writer.Flush()
	}
}

func (s *mockIMAPServer) GetAuthAttempts() int {
	return int(atomic.LoadInt32(&s.authAttempts))
}

func (s *mockIMAPServer) ResetAuthAttempts() {
	atomic.StoreInt32(&s.authAttempts, 0)
}

func (s *mockIMAPServer) Close() {
	s.listener.Close()
}

func (s *mockIMAPServer) GetHost() string {
	host, _, _ := net.SplitHostPort(s.address)
	return host
}

func (s *mockIMAPServer) GetPort() int {
	_, portStr, _ := net.SplitHostPort(s.address)
	var port int
	fmt.Sscanf(portStr, "%d", &port)
	return port
}

// generateSelfSignedCertificate generates a self-signed certificate for testing
func generateSelfSignedCertificate() (tls.Certificate, error) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return tls.Certificate{}, err
	}

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"Test Co"},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IPAddresses:           []net.IP{net.IPv4(127, 0, 0, 1)},
	}

	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		return tls.Certificate{}, err
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)})

	return tls.X509KeyPair(certPEM, keyPEM)
}

// TestAuthenticationNoRecursion verifies that authentication failures don't cause recursion
func TestAuthenticationNoRecursion(t *testing.T) {
	// Save original settings
	originalVerbose := Verbose
	originalRetryCount := RetryCount
	originalTLSSkipVerify := TLSSkipVerify

	// Configure for testing
	Verbose = false
	RetryCount = 3       // Set retry count to verify it's not used for auth
	TLSSkipVerify = true // Skip verification for test cert

	defer func() {
		Verbose = originalVerbose
		RetryCount = originalRetryCount
		TLSSkipVerify = originalTLSSkipVerify
	}()

	server, err := newMockIMAPServer("testuser", "testpass")
	if err != nil {
		t.Fatalf("Failed to create mock server: %v", err)
	}
	defer server.Close()

	// Test 1: Successful authentication
	t.Run("SuccessfulAuth", func(t *testing.T) {
		server.ResetAuthAttempts()

		d, err := New("testuser", "testpass", server.GetHost(), server.GetPort())
		if err != nil {
			t.Errorf("Expected successful connection, got error: %v", err)
		}
		if d != nil {
			d.Close()
		}

		// Should only attempt auth once
		if attempts := server.GetAuthAttempts(); attempts != 1 {
			t.Errorf("Expected 1 auth attempt, got %d", attempts)
		}
	})

	// Test 2: Failed authentication should not retry
	t.Run("FailedAuthNoRetry", func(t *testing.T) {
		server.ResetAuthAttempts()

		// Use a channel to detect if the function returns in reasonable time
		done := make(chan bool, 1)
		var connErr error

		go func() {
			_, connErr = New("testuser", "wrongpass", server.GetHost(), server.GetPort())
			done <- true
		}()

		select {
		case <-done:
			// Good, function returned
			if connErr == nil {
				t.Error("Expected authentication error, got nil")
			}
		case <-time.After(2 * time.Second):
			t.Error("Authentication appears to be stuck in recursion")
		}

		// Should only attempt auth once despite RetryCount being 3
		attempts := server.GetAuthAttempts()
		if attempts != 1 {
			t.Errorf("Expected 1 auth attempt (no retry), got %d", attempts)
		}
	})

	// Test 3: XOAUTH2 authentication should also not retry
	t.Run("XOAuth2NoRetry", func(t *testing.T) {
		server.ResetAuthAttempts()
		server.failAuth = true
		defer func() { server.failAuth = false }()

		done := make(chan bool, 1)
		var connErr error

		go func() {
			_, connErr = NewWithOAuth2("testuser", "token", server.GetHost(), server.GetPort())
			done <- true
		}()

		select {
		case <-done:
			if connErr == nil {
				t.Error("Expected authentication error, got nil")
			}
		case <-time.After(2 * time.Second):
			t.Error("XOAUTH2 authentication appears to be stuck in recursion")
		}

		// Should only attempt auth once
		attempts := server.GetAuthAttempts()
		if attempts != 1 {
			t.Errorf("Expected 1 XOAUTH2 auth attempt (no retry), got %d", attempts)
		}
	})
}

// TestConnectionRetry verifies that connection failures still retry
func TestConnectionRetry(t *testing.T) {
	// Save original settings
	originalVerbose := Verbose
	originalRetryCount := RetryCount
	originalTLSSkipVerify := TLSSkipVerify

	// Configure for testing
	Verbose = false
	RetryCount = 2 // Reduce for faster test
	TLSSkipVerify = true

	defer func() {
		Verbose = originalVerbose
		RetryCount = originalRetryCount
		TLSSkipVerify = originalTLSSkipVerify
	}()

	// Test connecting to a port that's not listening
	// This should retry according to RetryCount
	start := time.Now()
	_, err := New("user", "pass", "127.0.0.1", 59999) // Use unlikely port
	elapsed := time.Since(start)

	if err == nil {
		t.Error("Expected connection error, got nil")
	}

	// With retries, it should take some time (but not too long)
	// Each retry has a delay, so it should take at least a second
	if elapsed < 100*time.Millisecond {
		t.Error("Connection failed too quickly, retries may not be working")
	}

	// But it shouldn't take forever (indicates no infinite loop)
	if elapsed > 30*time.Second {
		t.Error("Connection took too long, possible infinite loop")
	}
}

// TestReconnectWithBadCredentials verifies Reconnect handles auth failures properly
func TestReconnectWithBadCredentials(t *testing.T) {
	// Save original settings
	originalVerbose := Verbose
	originalTLSSkipVerify := TLSSkipVerify

	// Configure for testing
	Verbose = false
	TLSSkipVerify = true

	defer func() {
		Verbose = originalVerbose
		TLSSkipVerify = originalTLSSkipVerify
	}()

	server, err := newMockIMAPServer("testuser", "testpass")
	if err != nil {
		t.Fatalf("Failed to create mock server: %v", err)
	}
	defer server.Close()

	// Create a connection with good credentials
	d, err := New("testuser", "testpass", server.GetHost(), server.GetPort())
	if err != nil {
		t.Fatalf("Failed to create initial connection: %v", err)
	}
	defer d.Close()

	// Change password to simulate bad credentials on reconnect
	d.Password = "wrongpass"
	server.ResetAuthAttempts()

	// Attempt reconnect with bad credentials
	err = d.Reconnect()
	if err == nil {
		t.Error("Expected reconnect to fail with bad credentials")
	}

	// Should only attempt auth once
	attempts := server.GetAuthAttempts()
	if attempts != 1 {
		t.Errorf("Expected 1 auth attempt on reconnect, got %d", attempts)
	}

	// Connection should be closed after failed auth
	if d.Connected {
		t.Error("Connection should be closed after failed reconnect")
	}
}

// TestSimpleAuthRecursionCheck does a simple test without mock server
func TestSimpleAuthRecursionCheck(t *testing.T) {
	// Save original settings
	originalVerbose := Verbose
	originalRetryCount := RetryCount
	originalDialTimeout := DialTimeout

	// Configure for testing
	Verbose = false
	RetryCount = 2                // Reduce retry count for faster test
	DialTimeout = 1 * time.Second // Set short timeout to avoid long waits

	defer func() {
		Verbose = originalVerbose
		RetryCount = originalRetryCount
		DialTimeout = originalDialTimeout
	}()

	// Try to connect to localhost on a port that's definitely not listening
	// This should fail quickly and retry according to RetryCount
	start := time.Now()
	_, err := New("test", "test", "127.0.0.1", 54321) // Use localhost with random port
	elapsed := time.Since(start)

	if err == nil {
		t.Error("Expected error connecting to non-listening port")
	}

	// Connection failure should retry, so it should take more than immediate
	if elapsed < 100*time.Millisecond {
		t.Error("Failed too quickly, connection retry might not be working")
	}

	// Should complete within reasonable time (not stuck in recursion)
	// With 2 retries and 1 second timeout, should be done in under 10 seconds
	if elapsed > 10*time.Second {
		t.Error("Took too long, might be stuck in recursion")
	}
}
