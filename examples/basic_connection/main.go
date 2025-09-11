package main

import (
	"fmt"
	"log"
	"time"

	imap "github.com/BrianLeishman/go-imap"
)

func main() {
	// Optional configuration
	imap.Verbose = false                   // Set to true to see all IMAP commands/responses
	imap.RetryCount = 3                    // Number of retries for failed commands
	imap.DialTimeout = 10 * time.Second    // Connection timeout
	imap.CommandTimeout = 30 * time.Second // Command timeout

	// For self-signed certificates (use with caution!)
	// imap.TLSSkipVerify = true

	// Connect with standard LOGIN authentication
	// Replace with your actual credentials and server
	m, err := imap.New("username", "password", "mail.server.com", 993)
	if err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}
	defer m.Close()

	// Quick test - list folders
	folders, err := m.GetFolders()
	if err != nil {
		log.Fatalf("Failed to get folders: %v", err)
	}

	fmt.Printf("Connected! Found %d folders\n", len(folders))
	fmt.Println("\nAvailable folders:")
	for _, folder := range folders {
		fmt.Printf("  - %s\n", folder)
	}
}
