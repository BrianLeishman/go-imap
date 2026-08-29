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
	imap.Verbose = false // Set to true to see all IMAP commands/responses

	// For self-signed certificates (use with caution!)
	// imap.TLSSkipVerify = true

	// Connect with standard LOGIN authentication.
	// Replace with your actual credentials and server.
	m, err := imap.Dial(context.Background(), imap.Options{
		Host:           "mail.server.com",
		Port:           993,
		Auth:           imap.PasswordAuth{Username: "username", Password: "password"},
		DialTimeout:    10 * time.Second,
		CommandTimeout: 30 * time.Second,
		RetryCount:     3,
	})
	if err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}
	defer func() {
		if err := m.Close(); err != nil {
			log.Printf("Failed to close connection: %v", err)
		}
	}()

	// Quick test - list folders
	folders, err := m.GetFolders(ctx)
	if err != nil {
		log.Fatalf("Failed to get folders: %v", err)
	}

	fmt.Printf("Connected! Found %d folders\n", len(folders))
	fmt.Println("\nAvailable folders:")
	for _, folder := range folders {
		fmt.Printf("  - %s\n", folder)
	}
}
