package main

import (
	"fmt"
	"log"

	imap "github.com/BrianLeishman/go-imap"
)

func main() {
	// Connect to server
	m, err := imap.New("username", "password", "mail.server.com", 993)
	if err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}
	defer m.Close()

	// List all folders
	folders, err := m.GetFolders()
	if err != nil {
		log.Fatalf("Failed to get folders: %v", err)
	}

	fmt.Println("Available folders:")
	for _, folder := range folders {
		fmt.Printf("  - %s\n", folder)
	}
	// Example output:
	// - INBOX
	// - Sent
	// - Drafts
	// - Trash
	// - INBOX/Receipts
	// - INBOX/Important
	// - [Gmail]/All Mail
	// - [Gmail]/Spam

	fmt.Println("\n--- Folder Operations ---")

	// Select a folder for operations (read-write mode)
	err = m.SelectFolder("INBOX")
	if err != nil {
		log.Fatalf("Failed to select INBOX: %v", err)
	}
	fmt.Println("Selected INBOX in read-write mode")

	// Get message count in current folder
	allUIDs, err := m.GetUIDs("ALL")
	if err != nil {
		log.Fatalf("Failed to get message count: %v", err)
	}
	fmt.Printf("INBOX contains %d messages\n", len(allUIDs))

	// Select folder in read-only mode
	err = m.ExamineFolder("Sent")
	if err != nil {
		log.Fatalf("Failed to examine Sent folder: %v", err)
	}
	fmt.Println("\nExamined Sent folder in read-only mode")

	sentUIDs, err := m.GetUIDs("ALL")
	if err != nil {
		log.Fatalf("Failed to get sent message count: %v", err)
	}
	fmt.Printf("Sent folder contains %d messages\n", len(sentUIDs))

	fmt.Println("\n--- Email Counts ---")

	// Get total email count across all folders
	totalCount, err := m.GetTotalEmailCount()
	if err != nil {
		log.Fatalf("Failed to get total email count: %v", err)
	}
	fmt.Printf("Total emails in all folders: %d\n", totalCount)

	// Get count excluding certain folders
	excludedFolders := []string{"Trash", "[Gmail]/Spam", "Junk", "Deleted"}
	count, err := m.GetTotalEmailCountExcluding(excludedFolders)
	if err != nil {
		log.Fatalf("Failed to get filtered email count: %v", err)
	}
	fmt.Printf("Total emails (excluding spam/trash): %d\n", count)

	// Calculate percentage
	if totalCount > 0 {
		percentage := float64(count) / float64(totalCount) * 100
		fmt.Printf("That's %.1f%% of your total emails\n", percentage)
	}
}
