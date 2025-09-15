package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	imap "github.com/BrianLeishman/go-imap"
)

func main() {
	// Connect to server
	m, err := imap.New("username", "password", "mail.server.com", 993)
	if err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}
	defer func() {
		if err := m.Close(); err != nil {
			log.Printf("Failed to close connection: %v", err)
		}
	}()

	// Select the folder to monitor
	if err := m.SelectFolder("INBOX"); err != nil {
		log.Fatalf("Failed to select INBOX: %v", err)
	}

	fmt.Println("=== IDLE Monitoring Example ===")
	fmt.Println("IDLE allows real-time notifications when mailbox changes occur")
	fmt.Println("The connection will automatically refresh IDLE every 5 minutes (RFC requirement)")
	fmt.Println()

	// Create an event handler
	handler := &imap.IdleHandler{
		// New email arrived
		OnExists: func(e imap.ExistsEvent) {
			fmt.Printf("[EXISTS] New message arrived! Message index: %d\n", e.MessageIndex)
			fmt.Printf("         Timestamp: %s\n", time.Now().Format("15:04:05"))

			// You might want to fetch the new email
			// Note: MessageIndex is the sequence number, not UID
			fmt.Printf("         Fetching new email details...\n")

			// Convert sequence number to UID
			uids, err := m.GetUIDs(fmt.Sprintf("%d", e.MessageIndex))
			if err == nil && len(uids) > 0 {
				uid := uids[0]
				overviews, err := m.GetOverviews(uid)
				if err == nil && len(overviews) > 0 {
					email := overviews[uid]
					fmt.Printf("         Subject: %s\n", email.Subject)
					fmt.Printf("         From: %s\n", email.From)
					fmt.Printf("         Size: %.1f KB\n", float64(email.Size)/1024)
				}
			}
			fmt.Println()
		},

		// Email was deleted/expunged
		OnExpunge: func(e imap.ExpungeEvent) {
			fmt.Printf("[EXPUNGE] Message removed at index: %d\n", e.MessageIndex)
			fmt.Printf("          Timestamp: %s\n", time.Now().Format("15:04:05"))
			fmt.Println()
		},

		// Email flags changed (read, flagged, etc.)
		OnFetch: func(e imap.FetchEvent) {
			fmt.Printf("[FETCH] Flags changed\n")
			fmt.Printf("        Message Index: %d\n", e.MessageIndex)
			fmt.Printf("        UID: %d\n", e.UID)
			fmt.Printf("        New Flags: %v\n", e.Flags)
			fmt.Printf("        Timestamp: %s\n", time.Now().Format("15:04:05"))

			// Interpret the flags
			flagDescriptions := []string{}
			for _, flag := range e.Flags {
				switch flag {
				case "\\Seen":
					flagDescriptions = append(flagDescriptions, "marked as read")
				case "\\Flagged":
					flagDescriptions = append(flagDescriptions, "starred/flagged")
				case "\\Answered":
					flagDescriptions = append(flagDescriptions, "marked as answered")
				case "\\Deleted":
					flagDescriptions = append(flagDescriptions, "marked for deletion")
				case "\\Draft":
					flagDescriptions = append(flagDescriptions, "marked as draft")
				}
			}
			if len(flagDescriptions) > 0 {
				fmt.Printf("        Interpretation: Email was %s\n", flagDescriptions)
			}
			fmt.Println()
		},
	}

	// Get initial state
	allUIDs, _ := m.GetUIDs("ALL")
	unseenUIDs, _ := m.GetUIDs("UNSEEN")
	fmt.Printf("Initial state: %d total emails, %d unread\n", len(allUIDs), len(unseenUIDs))
	fmt.Println()

	// Start IDLE (non-blocking, runs in background)
	fmt.Println("Starting IDLE monitoring...")
	fmt.Println("Press Ctrl+C to stop")
	fmt.Println("Try these actions in your email client to see events:")
	fmt.Println("  - Send yourself an email")
	fmt.Println("  - Mark an email as read/unread")
	fmt.Println("  - Star/flag an email")
	fmt.Println("  - Delete an email")
	fmt.Println()

	err = m.StartIdle(handler)
	if err != nil {
		log.Fatalf("Failed to start IDLE: %v", err)
	}

	// Set up signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Also set up a timer for demo purposes (30 minutes max)
	timer := time.NewTimer(30 * time.Minute)

	// Wait for interrupt or timeout
	select {
	case <-sigChan:
		fmt.Println("\nInterrupt received, stopping IDLE...")
	case <-timer.C:
		fmt.Println("\n30 minute demo timeout reached, stopping IDLE...")
	}

	// Stop IDLE monitoring
	err = m.StopIdle()
	if err != nil {
		log.Printf("Error stopping IDLE: %v", err)
	} else {
		fmt.Println("IDLE monitoring stopped successfully")
	}

	// Get final state
	allUIDs, _ = m.GetUIDs("ALL")
	unseenUIDs, _ = m.GetUIDs("UNSEEN")
	fmt.Printf("\nFinal state: %d total emails, %d unread\n", len(allUIDs), len(unseenUIDs))
}
