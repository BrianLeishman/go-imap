package main

import (
	"context"
	"fmt"
	"log"
	"time"

	imap "github.com/BrianLeishman/go-imap"
)

var ctx = context.Background()

func main() {
	fmt.Println("=== Error Handling and Reconnection Example ===")

	// Example 1: Handle connection errors
	handleConnectionErrors()

	// Example 2: Robust email fetching with automatic retry
	robustEmailFetch()

	// Example 3: Manual reconnection
	manualReconnection()

	// Example 4: Timeout configuration
	timeoutConfiguration()
}

// baseOptions returns the options used across examples. Replace host, port,
// and credentials with your own.
func baseOptions(username, password, host string) imap.Options {
	return imap.Options{
		Host:           host,
		Port:           993,
		Auth:           imap.PasswordAuth{Username: username, Password: password},
		DialTimeout:    10 * time.Second,
		CommandTimeout: 30 * time.Second,
		RetryCount:     5,
	}
}

func handleConnectionErrors() {
	fmt.Println("1. Handling Connection Errors")
	fmt.Println("------------------------------")
	ctx := context.Background()

	// Try to connect with invalid credentials (will fail)
	m, err := imap.Dial(ctx, baseOptions("invalid-user", "invalid-pass", "imap.gmail.com"))
	if err != nil {
		fmt.Printf("Expected error occurred: %v\n", err)
		fmt.Println("This is how you handle initial connection failures")
	} else {
		defer func() {
			if err := m.Close(); err != nil {
				log.Printf("Failed to close connection: %v", err)
			}
		}()
		fmt.Println("Unexpected: connection succeeded with invalid credentials")
	}

	// Try to connect to non-existent server
	m, err = imap.Dial(ctx, baseOptions("user", "pass", "non-existent-server.invalid"))
	if err != nil {
		fmt.Printf("Expected error for invalid server: %v\n", err)
	} else {
		defer func() {
			if err := m.Close(); err != nil {
				log.Printf("Failed to close connection: %v", err)
			}
		}()
	}

	fmt.Println()
}

func robustEmailFetch() {
	fmt.Println("2. Robust Email Fetching")
	fmt.Println("-------------------------")

	ctx := context.Background()
	// NOTE: Replace with your actual credentials and server
	m, err := imap.Dial(ctx, baseOptions("username", "password", "mail.server.com"))
	if err != nil {
		fmt.Printf("Failed to connect: %v\n", err)
		fmt.Println("Skipping robust fetch example (need valid credentials)")
		fmt.Println()
		return
	}
	defer func() {
		if err := m.Close(); err != nil {
			log.Printf("Failed to close connection: %v", err)
		}
	}()

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
	err = m.SelectFolder(ctx, "INBOX")
	if err != nil {
		fmt.Printf("Failed to select folder: %v\n", err)
		return
	}
	fmt.Println("Selected INBOX successfully")

	// Fetch emails with automatic retry on network issues
	uids, err := m.GetUIDs(ctx, "1:5")
	if err != nil {
		fmt.Printf("Search failed after retries: %v\n", err)
		return
	}
	fmt.Printf("Found %d UIDs with automatic retry support\n", len(uids))

	if len(uids) > 0 {
		emails, err := m.GetEmails(ctx, uids[0])
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
	fmt.Println("3. Manual Reconnection")
	fmt.Println("-----------------------")

	ctx := context.Background()
	m, err := imap.Dial(ctx, baseOptions("username", "password", "mail.server.com"))
	if err != nil {
		fmt.Printf("Failed to connect: %v\n", err)
		fmt.Println("Skipping manual reconnection example (need valid credentials)")
		fmt.Println()
		return
	}
	defer func() {
		if err := m.Close(); err != nil {
			log.Printf("Failed to close connection: %v", err)
		}
	}()

	fmt.Println("Connected successfully!")

	err = m.SelectFolder(ctx, "INBOX")
	if err != nil {
		fmt.Printf("Failed to select folder: %v\n", err)
		fmt.Println("Attempting manual reconnection...")
		if err := m.Reconnect(ctx); err != nil {
			fmt.Printf("Manual reconnection failed: %v\n", err)
			return
		}
		fmt.Println("Reconnected successfully!")
		if err := m.SelectFolder(ctx, "INBOX"); err != nil {
			fmt.Printf("Failed to select folder after reconnect: %v\n", err)
			return
		}
		fmt.Println("Selected INBOX after reconnection")
	}

	fmt.Println()
}

func timeoutConfiguration() {
	fmt.Println("4. Timeout Configuration")
	fmt.Println("-------------------------")

	// Use aggressive per-call timeouts for demonstration.
	opts := imap.Options{
		Host:           "slow-server.example.com",
		Port:           993,
		Auth:           imap.PasswordAuth{Username: "user", Password: "pass"},
		DialTimeout:    2 * time.Second, // Very short connection timeout
		CommandTimeout: 5 * time.Second, // Short command timeout
	}
	fmt.Printf("Using aggressive timeouts: DialTimeout=%v, CommandTimeout=%v\n",
		opts.DialTimeout, opts.CommandTimeout)

	start := time.Now()
	m, err := imap.Dial(context.Background(), opts)
	elapsed := time.Since(start)

	if err != nil {
		fmt.Printf("Connection failed after %v: %v\n", elapsed, err)
		fmt.Println("This demonstrates how DialTimeout works")
	} else {
		defer func() {
			if err := m.Close(); err != nil {
				log.Printf("Failed to close connection: %v", err)
			}
		}()

		fmt.Println("Attempting a command that might timeout...")
		start = time.Now()
		_, err := m.GetUIDs(ctx, "ALL")
		elapsed = time.Since(start)

		if err != nil {
			fmt.Printf("Command failed after %v: %v\n", elapsed, err)
		} else {
			fmt.Println("Command completed successfully")
		}
	}

	fmt.Println()
	fmt.Println("=== Best Practices for Error Handling ===")
	fmt.Println("1. Set a reasonable RetryCount for transient failures")
	fmt.Println("2. Use sensible timeouts (10-30 seconds typically)")
	fmt.Println("3. Always check errors from operations")
	fmt.Println("4. Consider manual reconnection for critical operations")
	fmt.Println("5. Enable imap.Verbose when debugging issues")
	fmt.Println("6. Log errors for monitoring and debugging")
}
