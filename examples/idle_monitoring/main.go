package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	imap "github.com/BrianLeishman/go-imap"
)

var ctx = context.Background()

func connectAndSelect() (*imap.Client, error) {
	m, err := imap.Dial(context.Background(), imap.Options{
		Host: "mail.server.com",
		Port: 993,
		Auth: imap.PasswordAuth{Username: "username", Password: "password"},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect: %w", err)
	}

	if err := m.SelectFolder(ctx, "INBOX"); err != nil {
		_ = m.Close()
		return nil, fmt.Errorf("failed to select INBOX: %w", err)
	}

	return m, nil
}

func handleNewEmail(m *imap.Client, e imap.ExistsEvent) {
	fmt.Printf("[EXISTS] New message arrived! Sequence number: %d\n", e.SeqNum)
	fmt.Printf("         Timestamp: %s\n", time.Now().Format("15:04:05"))
	fmt.Printf("         Fetching new email details...\n")

	// Resolve sequence number -> UID via SEARCH (a bare number in IMAP
	// SEARCH matches by sequence number). EXISTS reports the new total
	// count, which equals the sequence number of the newest message.
	uids, err := m.GetUIDs(ctx, fmt.Sprintf("%d", e.SeqNum))
	if err != nil || len(uids) == 0 {
		fmt.Println()
		return
	}

	uid := uids[0]
	overviews, err := m.GetOverviews(ctx, uid)
	if err == nil && len(overviews) > 0 {
		email := overviews[uid]
		fmt.Printf("         Subject: %s\n", email.Subject)
		fmt.Printf("         From: %s\n", email.From)
		fmt.Printf("         Size: %.1f KB\n", float64(email.Size)/1024)
	}
	fmt.Println()
}

func describeFlagChange(flags []string) {
	flagDescriptions := []string{}
	for _, flag := range flags {
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
}

func createIdleHandler(m *imap.Client) *imap.IdleHandler {
	return &imap.IdleHandler{
		OnExists: func(e imap.ExistsEvent) {
			handleNewEmail(m, e)
		},

		OnExpunge: func(e imap.ExpungeEvent) {
			fmt.Printf("[EXPUNGE] Message removed at sequence number: %d\n", e.SeqNum)
			fmt.Printf("          Timestamp: %s\n", time.Now().Format("15:04:05"))
			fmt.Println()
		},

		OnFetch: func(e imap.FetchEvent) {
			fmt.Printf("[FETCH] Flags changed\n")
			fmt.Printf("        Sequence Number: %d\n", e.SeqNum)
			fmt.Printf("        UID: %d\n", e.UID)
			fmt.Printf("        New Flags: %v\n", e.Flags)
			fmt.Printf("        Timestamp: %s\n", time.Now().Format("15:04:05"))
			describeFlagChange(e.Flags)
			fmt.Println()
		},
	}
}

func printInitialState(m *imap.Client) {
	allUIDs, _ := m.GetUIDs(ctx, "ALL")
	unseenUIDs, _ := m.GetUIDs(ctx, "UNSEEN")
	fmt.Printf("Initial state: %d total emails, %d unread\n", len(allUIDs), len(unseenUIDs))
	fmt.Println()
}

func printIdleInstructions() {
	fmt.Println("Starting IDLE monitoring...")
	fmt.Println("Press Ctrl+C to stop")
	fmt.Println("Try these actions in your email client to see events:")
	fmt.Println("  - Send yourself an email")
	fmt.Println("  - Mark an email as read/unread")
	fmt.Println("  - Star/flag an email")
	fmt.Println("  - Delete an email")
	fmt.Println()
}

func waitForShutdown() {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	timer := time.NewTimer(30 * time.Minute)

	select {
	case <-sigChan:
		fmt.Println("\nInterrupt received, stopping IDLE...")
	case <-timer.C:
		fmt.Println("\n30 minute demo timeout reached, stopping IDLE...")
	}
}

func main() {
	fmt.Println("=== IDLE Monitoring Example ===")
	fmt.Println("IDLE allows real-time notifications when mailbox changes occur")
	fmt.Println("The connection will automatically refresh IDLE every 5 minutes (RFC requirement)")
	fmt.Println()

	m, err := connectAndSelect()
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		if err := m.Close(); err != nil {
			log.Printf("Failed to close connection: %v", err)
		}
	}()

	printInitialState(m)
	printIdleInstructions()

	handler := createIdleHandler(m)
	err = m.StartIdle(ctx, handler)
	if err != nil {
		log.Fatalf("Failed to start IDLE: %v", err)
	}

	waitForShutdown()

	err = m.StopIdle()
	if err != nil {
		log.Printf("Error stopping IDLE: %v", err)
	} else {
		fmt.Println("IDLE monitoring stopped successfully")
	}

	// Get final state
	allUIDs, _ := m.GetUIDs(ctx, "ALL")
	unseenUIDs, _ := m.GetUIDs(ctx, "UNSEEN")
	fmt.Printf("\nFinal state: %d total emails, %d unread\n", len(allUIDs), len(unseenUIDs))
}
