package main

import (
	"fmt"
	"log"
	"time"

	imap "github.com/BrianLeishman/go-imap"
)

func initializeClient() (*imap.Dialer, error) {
	imap.Verbose = false
	imap.RetryCount = 3
	imap.DialTimeout = 10 * time.Second
	imap.CommandTimeout = 30 * time.Second

	fmt.Println("Connecting to IMAP server...")
	// NOTE: Replace with your actual credentials and server
	m, err := imap.New("your-email@gmail.com", "your-password", "imap.gmail.com", 993)
	if err != nil {
		return nil, fmt.Errorf("connection failed: %w", err)
	}
	return m, nil
}

func listAvailableFolders(m *imap.Dialer) error {
	fmt.Println("\nAvailable folders:")
	folders, err := m.GetFolders()
	if err != nil {
		return fmt.Errorf("failed to get folders: %w", err)
	}
	for _, folder := range folders {
		fmt.Printf("  - %s\n", folder)
	}
	return nil
}

func fetchUnreadEmails(m *imap.Dialer) ([]int, error) {
	fmt.Println("\nSelecting INBOX...")
	if err := m.SelectFolder("INBOX"); err != nil {
		return nil, fmt.Errorf("failed to select INBOX: %w", err)
	}

	fmt.Println("\nSearching for unread emails...")
	unreadUIDs, err := m.GetUIDs("UNSEEN")
	if err != nil {
		return nil, fmt.Errorf("search failed: %w", err)
	}
	fmt.Printf("Found %d unread emails\n", len(unreadUIDs))
	return unreadUIDs, nil
}

func displayEmails(m *imap.Dialer, unreadUIDs []int) {
	limit := 5
	if len(unreadUIDs) < limit {
		limit = len(unreadUIDs)
	}

	if limit == 0 {
		return
	}

	fmt.Printf("\nFetching first %d unread emails...\n", limit)
	emails, err := m.GetEmails(unreadUIDs[:limit]...)
	if err != nil {
		log.Fatalf("Failed to fetch emails: %v", err)
	}

	for uid, email := range emails {
		printEmailDetails(uid, email)

		if uid == unreadUIDs[0] {
			markFirstAsRead(m, uid)
		}
	}
}

func printEmailDetails(uid int, email *imap.Email) {
	fmt.Printf("\n--- Email UID %d ---\n", uid)
	fmt.Printf("From: %s\n", email.From)
	fmt.Printf("Subject: %s\n", email.Subject)
	fmt.Printf("Date: %s\n", email.Sent.Format("Jan 2, 2006 3:04 PM"))
	fmt.Printf("Size: %.1f KB\n", float64(email.Size)/1024)

	if len(email.Text) > 100 {
		fmt.Printf("Preview: %.100s...\n", email.Text)
	} else if len(email.Text) > 0 {
		fmt.Printf("Preview: %s\n", email.Text)
	}

	if len(email.Attachments) > 0 {
		fmt.Printf("Attachments: %d\n", len(email.Attachments))
		for _, att := range email.Attachments {
			fmt.Printf("  - %s (%.1f KB)\n", att.Name, float64(len(att.Content))/1024)
		}
	}
}

func markFirstAsRead(m *imap.Dialer, uid int) {
	fmt.Printf("\nMarking email %d as read...\n", uid)
	if err := m.MarkSeen(uid); err != nil {
		fmt.Printf("Failed to mark as read: %v\n", err)
	}
}

func printMailboxStats(m *imap.Dialer) {
	fmt.Println("\nMailbox Statistics:")
	allUIDs, _ := m.GetUIDs("ALL")
	seenUIDs, _ := m.GetUIDs("SEEN")
	flaggedUIDs, _ := m.GetUIDs("FLAGGED")

	fmt.Printf("  Total emails: %d\n", len(allUIDs))
	fmt.Printf("  Read emails: %d\n", len(seenUIDs))
	fmt.Printf("  Unread emails: %d\n", len(allUIDs)-len(seenUIDs))
	fmt.Printf("  Flagged emails: %d\n", len(flaggedUIDs))
}

func monitorBriefly(m *imap.Dialer) {
	fmt.Println("\nMonitoring for new emails (10 seconds)...")
	handler := &imap.IdleHandler{
		OnExists: func(e imap.ExistsEvent) {
			fmt.Printf("  New email arrived! (message #%d)\n", e.MessageIndex)
		},
	}

	if err := m.StartIdle(handler); err == nil {
		time.Sleep(10 * time.Second)
		_ = m.StopIdle()
	}
}

func main() {
	m, err := initializeClient()
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		if err := m.Close(); err != nil {
			log.Printf("Failed to close connection: %v", err)
		}
	}()

	if err := listAvailableFolders(m); err != nil {
		log.Fatal(err)
	}

	unreadUIDs, err := fetchUnreadEmails(m)
	if err != nil {
		log.Fatal(err)
	}

	displayEmails(m, unreadUIDs)
	printMailboxStats(m)
	monitorBriefly(m)

	fmt.Println("\nDone!")
}

/* Example Output:

Connecting to IMAP server...

Available folders:
  - INBOX
  - Sent
  - Drafts
  - Trash
  - [Gmail]/All Mail
  - [Gmail]/Spam
  - [Gmail]/Starred
  - [Gmail]/Important

Selecting INBOX...

Searching for unread emails...
Found 3 unread emails

Fetching first 3 unread emails...

--- Email UID 1247 ---
From: notifications@github.com:GitHub
Subject: [org/repo] New issue: Bug in authentication flow (#123)
Date: Nov 11, 2024 2:15 PM
Size: 8.5 KB
Preview: User johndoe opened an issue: When trying to authenticate with OAuth2, the system returns a 401 error even with valid...
Attachments: 0

Marking email 1247 as read...

--- Email UID 1248 ---
From: team@company.com:Team Update
Subject: Weekly Team Sync - Meeting Notes
Date: Nov 11, 2024 3:30 PM
Size: 12.3 KB
Preview: Hi team, Here are the notes from today's sync: 1. Project Alpha is on track for Dec release 2. Need volunteers for...
Attachments: 1
  - meeting-notes.pdf (156.2 KB)

--- Email UID 1249 ---
From: noreply@service.com:Service Alert
Subject: Your monthly report is ready
Date: Nov 11, 2024 4:45 PM
Size: 45.6 KB
Preview: Your monthly usage report for October 2024 is now available. View it in your dashboard or download the attached PDF...
Attachments: 2
  - october-report.pdf (523.1 KB)
  - usage-chart.png (89.3 KB)

Mailbox Statistics:
  Total emails: 1532
  Read emails: 1530
  Unread emails: 2
  Flagged emails: 23

Monitoring for new emails (10 seconds)...

Done!
*/
