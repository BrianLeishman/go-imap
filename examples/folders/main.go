package main

import (
	"context"
	"fmt"
	"log"

	imap "github.com/BrianLeishman/go-imap"
)

var ctx = context.Background()

func connectToServer() (*imap.Client, error) {
	m, err := imap.Dial(context.Background(), imap.Options{
		Host: "mail.server.com",
		Port: 993,
		Auth: imap.PasswordAuth{Username: "username", Password: "password"},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect: %w", err)
	}
	return m, nil
}

func listFolders(m *imap.Client) error {
	folders, err := m.GetFolders(ctx)
	if err != nil {
		return fmt.Errorf("failed to get folders: %w", err)
	}

	fmt.Println("Available folders:")
	for _, folder := range folders {
		fmt.Printf("  - %s\n", folder)
	}
	return nil
}

func demonstrateFolderSelection(m *imap.Client) error {
	fmt.Println("\n--- Folder Operations ---")

	// Select a folder for operations (read-write mode)
	err := m.SelectFolder(ctx, "INBOX")
	if err != nil {
		return fmt.Errorf("failed to select INBOX: %w", err)
	}
	fmt.Println("Selected INBOX in read-write mode")

	// Get message count in current folder
	allUIDs, err := m.GetUIDs(ctx, "ALL")
	if err != nil {
		return fmt.Errorf("failed to get message count: %w", err)
	}
	fmt.Printf("INBOX contains %d messages\n", len(allUIDs))

	// Select folder in read-only mode
	err = m.ExamineFolder(ctx, "Sent")
	if err != nil {
		return fmt.Errorf("failed to examine Sent folder: %w", err)
	}
	fmt.Println("\nExamined Sent folder in read-only mode")

	sentUIDs, err := m.GetUIDs(ctx, "ALL")
	if err != nil {
		return fmt.Errorf("failed to get sent message count: %w", err)
	}
	fmt.Printf("Sent folder contains %d messages\n", len(sentUIDs))

	return nil
}

func demonstrateFolderManagement(m *imap.Client) {
	fmt.Println("\n--- Folder Management ---")

	// Create a new folder
	err := m.CreateFolder(ctx, "INBOX/TestFolder")
	if err != nil {
		log.Printf("Failed to create folder: %v", err)
	} else {
		fmt.Println("Created folder: INBOX/TestFolder")
	}

	// Rename the folder
	err = m.RenameFolder(ctx, "INBOX/TestFolder", "INBOX/RenamedFolder")
	if err != nil {
		log.Printf("Failed to rename folder: %v", err)
	} else {
		fmt.Println("Renamed folder: INBOX/TestFolder -> INBOX/RenamedFolder")
	}

	// Delete the folder
	err = m.DeleteFolder(ctx, "INBOX/RenamedFolder")
	if err != nil {
		log.Printf("Failed to delete folder: %v", err)
	} else {
		fmt.Println("Deleted folder: INBOX/RenamedFolder")
	}
}

func demonstrateEmailCounts(m *imap.Client) error {
	fmt.Println("\n--- Email Counts ---")

	// Get total email count across all folders (traditional approach)
	totalCount, err := m.GetTotalEmailCount(ctx)
	if err != nil {
		fmt.Printf("Traditional count failed: %v\n", err)
		fmt.Println("This might happen with Gmail or other providers that have inaccessible system folders")
	} else {
		fmt.Printf("Total emails in all folders: %d\n", totalCount)
	}

	// Get total email count with robust error handling
	safeCount, folderErrors, err := m.GetTotalEmailCountSafe(ctx)
	if err != nil {
		return fmt.Errorf("failed to get safe total email count: %w", err)
	}
	fmt.Printf("Total emails (safe count): %d\n", safeCount)

	if len(folderErrors) > 0 {
		fmt.Printf("Note: %d folders had errors:\n", len(folderErrors))
		for _, folderErr := range folderErrors {
			fmt.Printf("  - %v\n", folderErr)
		}
	}

	// Get count excluding certain folders (safe version)
	excludedFolders := []string{"Trash", "[Gmail]/Spam", "Junk", "Deleted"}
	count, folderErrors, err := m.GetTotalEmailCountSafeExcluding(ctx, excludedFolders)
	if err != nil {
		return fmt.Errorf("failed to get filtered email count: %w", err)
	}
	fmt.Printf("Total emails (excluding spam/trash): %d\n", count)

	if len(folderErrors) > 0 {
		fmt.Printf("Folders with errors during exclusion count: %d\n", len(folderErrors))
	}

	// Calculate percentage
	if safeCount > 0 {
		percentage := float64(count) / float64(safeCount) * 100
		fmt.Printf("That's %.1f%% of your total emails\n", percentage)
	}

	return nil
}

func demonstrateFolderStats(m *imap.Client) error {
	fmt.Println("\n--- Detailed Folder Statistics ---")

	stats, err := m.GetFolderStats(ctx)
	if err != nil {
		return fmt.Errorf("failed to get folder statistics: %w", err)
	}

	fmt.Printf("Found %d folders:\n", len(stats))
	successfulFolders := 0
	totalEmails := 0

	for _, stat := range stats {
		if stat.Error != nil {
			fmt.Printf("  %-30s [ERROR]: %v\n", stat.Name, stat.Error)
		} else {
			fmt.Printf("  %-30s %5d emails, max UID: %d\n", stat.Name, stat.Count, stat.MaxUID)
			successfulFolders++
			totalEmails += stat.Count
		}
	}

	fmt.Printf("\nSummary: %d/%d folders accessible, %d total emails\n",
		successfulFolders, len(stats), totalEmails)

	return nil
}

func main() {
	m, err := connectToServer()
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		if err := m.Close(); err != nil {
			log.Printf("Failed to close connection: %v", err)
		}
	}()

	if err := listFolders(m); err != nil {
		log.Fatal(err)
	}

	if err := demonstrateFolderSelection(m); err != nil {
		log.Fatal(err)
	}

	demonstrateFolderManagement(m)

	if err := demonstrateEmailCounts(m); err != nil {
		log.Fatal(err)
	}

	if err := demonstrateFolderStats(m); err != nil {
		log.Fatal(err)
	}
}
