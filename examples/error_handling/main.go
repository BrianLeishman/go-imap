package main

import (
	"fmt"
	"time"

	imap "github.com/BrianLeishman/go-imap"
)

func main() {
	fmt.Println("=== Error Handling and Reconnection Example ===\n")

	// Configure retry and timeout behavior
	configureLibrarySettings()

	// Example 1: Handle connection errors
	handleConnectionErrors()

	// Example 2: Robust email fetching with automatic retry
	robustEmailFetch()

	// Example 3: Manual reconnection
	manualReconnection()

	// Example 4: Timeout configuration
	timeoutConfiguration()
}

func configureLibrarySettings() {
	fmt.Println("1. Configuring Library Settings")
	fmt.Println("--------------------------------")

	// Set retry configuration
	imap.RetryCount = 5 // Will retry failed operations 5 times
	fmt.Printf("RetryCount set to: %d\n", imap.RetryCount)

	// Enable verbose mode to see what's happening
	imap.Verbose = false // Set to true to see all IMAP commands/responses
	fmt.Printf("Verbose mode: %v\n", imap.Verbose)

	// Configure timeouts
	imap.DialTimeout = 10 * time.Second    // Connection timeout
	imap.CommandTimeout = 30 * time.Second // Individual command timeout
	fmt.Printf("DialTimeout: %v\n", imap.DialTimeout)
	fmt.Printf("CommandTimeout: %v\n", imap.CommandTimeout)

	fmt.Println()
}

func handleConnectionErrors() {
	fmt.Println("2. Handling Connection Errors")
	fmt.Println("------------------------------")

	// Try to connect with invalid credentials (will fail)
	m, err := imap.New("invalid-user", "invalid-pass", "imap.gmail.com", 993)
	if err != nil {
		fmt.Printf("Expected error occurred: %v\n", err)
		fmt.Println("This is how you handle initial connection failures")
	} else {
		defer m.Close()
		fmt.Println("Unexpected: connection succeeded with invalid credentials")
	}

	// Try to connect to non-existent server
	m, err = imap.New("user", "pass", "non-existent-server.invalid", 993)
	if err != nil {
		fmt.Printf("Expected error for invalid server: %v\n", err)
	} else {
		defer m.Close()
	}

	fmt.Println()
}

func robustEmailFetch() {
	fmt.Println("3. Robust Email Fetching")
	fmt.Println("-------------------------")

	// NOTE: Replace with your actual credentials and server
	m, err := imap.New("username", "password", "mail.server.com", 993)
	if err != nil {
		fmt.Printf("Failed to connect: %v\n", err)
		fmt.Println("Skipping robust fetch example (need valid credentials)")
		fmt.Println()
		return
	}
	defer m.Close()

	// The library automatically handles reconnection for most operations
	fmt.Println("Connected successfully!")
	fmt.Println("The library will automatically:")
	fmt.Println("  1. Detect connection failures")
	fmt.Println("  2. Close the broken connection")
	fmt.Println("  3. Create a new connection")
	fmt.Println("  4. Re-authenticate (LOGIN or XOAUTH2)")
	fmt.Println("  5. Re-select the previously selected folder")
	fmt.Println("  6. Retry the failed command")
	fmt.Println()

	// Select folder with automatic retry
	err = m.SelectFolder("INBOX")
	if err != nil {
		// This only fails after all retries are exhausted
		fmt.Printf("Failed to select folder after %d retries: %v\n", imap.RetryCount, err)
		return
	}
	fmt.Println("Selected INBOX successfully")

	// Fetch emails with automatic retry on network issues
	uids, err := m.GetUIDs("1:5")
	if err != nil {
		fmt.Printf("Search failed after retries: %v\n", err)
		return
	}
	fmt.Printf("Found %d UIDs with automatic retry support\n", len(uids))

	if len(uids) > 0 {
		// Fetch emails - will automatically retry on failure
		emails, err := m.GetEmails(uids[0])
		if err != nil {
			fmt.Printf("Fetch failed after retries: %v\n", err)
		} else {
			fmt.Printf("Successfully fetched email with UID %d\n", uids[0])
			for uid, email := range emails {
				fmt.Printf("  Subject: %s\n", email.Subject)
				fmt.Printf("  From: %s\n", email.From)
				_ = uid
			}
		}
	}

	fmt.Println()
}

func manualReconnection() {
	fmt.Println("4. Manual Reconnection")
	fmt.Println("-----------------------")

	// NOTE: Replace with your actual credentials and server
	m, err := imap.New("username", "password", "mail.server.com", 993)
	if err != nil {
		fmt.Printf("Failed to connect: %v\n", err)
		fmt.Println("Skipping manual reconnection example (need valid credentials)")
		fmt.Println()
		return
	}
	defer m.Close()

	fmt.Println("Connected successfully!")

	// Select a folder
	err = m.SelectFolder("INBOX")
	if err != nil {
		fmt.Printf("Failed to select folder: %v\n", err)

		// You can manually trigger a reconnection
		fmt.Println("Attempting manual reconnection...")
		if err := m.Reconnect(); err != nil {
			fmt.Printf("Manual reconnection failed: %v\n", err)
			return
		}
		fmt.Println("Reconnected successfully!")

		// Try selecting folder again
		if err := m.SelectFolder("INBOX"); err != nil {
			fmt.Printf("Failed to select folder after reconnect: %v\n", err)
			return
		}
		fmt.Println("Selected INBOX after reconnection")
	}

	fmt.Println()
}

func timeoutConfiguration() {
	fmt.Println("5. Timeout Configuration")
	fmt.Println("-------------------------")

	// Configure aggressive timeouts for demonstration
	originalDialTimeout := imap.DialTimeout
	originalCommandTimeout := imap.CommandTimeout

	imap.DialTimeout = 2 * time.Second    // Very short connection timeout
	imap.CommandTimeout = 5 * time.Second // Short command timeout

	fmt.Printf("Using aggressive timeouts:\n")
	fmt.Printf("  DialTimeout: %v\n", imap.DialTimeout)
	fmt.Printf("  CommandTimeout: %v\n", imap.CommandTimeout)

	// Try to connect with short timeout
	start := time.Now()
	m, err := imap.New("user", "pass", "slow-server.example.com", 993)
	elapsed := time.Since(start)

	if err != nil {
		fmt.Printf("Connection failed after %v: %v\n", elapsed, err)
		fmt.Println("This demonstrates how DialTimeout works")
	} else {
		defer m.Close()

		// This search will timeout after CommandTimeout
		fmt.Println("Attempting a command that might timeout...")
		start = time.Now()
		_, err := m.GetUIDs("ALL")
		elapsed = time.Since(start)

		if err != nil {
			fmt.Printf("Command failed after %v: %v\n", elapsed, err)
		} else {
			fmt.Println("Command completed successfully")
		}
	}

	// Restore original timeouts
	imap.DialTimeout = originalDialTimeout
	imap.CommandTimeout = originalCommandTimeout

	fmt.Println()
	fmt.Println("=== Best Practices for Error Handling ===")
	fmt.Println("1. Set appropriate RetryCount based on your needs")
	fmt.Println("2. Use reasonable timeouts (10-30 seconds typically)")
	fmt.Println("3. Always check errors from operations")
	fmt.Println("4. Consider manual reconnection for critical operations")
	fmt.Println("5. Enable Verbose mode when debugging issues")
	fmt.Println("6. Log errors for monitoring and debugging")
	fmt.Println("7. Implement exponential backoff for custom retry logic")
}

// Example of custom retry logic with exponential backoff
func customRetryLogic(m *imap.Dialer) error {
	maxRetries := 3
	baseDelay := time.Second

	var lastErr error
	for i := 0; i < maxRetries; i++ {
		if i > 0 {
			// Exponential backoff
			delay := baseDelay * time.Duration(1<<uint(i-1))
			fmt.Printf("Retry %d/%d after %v delay...\n", i+1, maxRetries, delay)
			time.Sleep(delay)
		}

		// Try the operation
		err := m.SelectFolder("INBOX")
		if err == nil {
			fmt.Println("Operation succeeded!")
			return nil
		}

		lastErr = err
		fmt.Printf("Attempt %d failed: %v\n", i+1, err)

		// Try to reconnect before next retry
		if i < maxRetries-1 {
			if reconnectErr := m.Reconnect(); reconnectErr != nil {
				fmt.Printf("Reconnection failed: %v\n", reconnectErr)
			}
		}
	}

	return fmt.Errorf("operation failed after %d retries: %w", maxRetries, lastErr)
}
