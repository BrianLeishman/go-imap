package main

import (
	"context"
	"fmt"
	"log"
	"strings"

	imap "github.com/BrianLeishman/go-imap"
)

var ctx = context.Background()

func connectAndSelectInbox() (*imap.Client, error) {
	m, err := imap.Dial(context.Background(), imap.Options{
		Host: "mail.server.com",
		Port: 993,
		Auth: imap.PasswordAuth{Username: "username", Password: "password"},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect: %w", err)
	}

	err = m.SelectFolder(ctx, "INBOX")
	if err != nil {
		_ = m.Close()
		return nil, fmt.Errorf("failed to select INBOX: %w", err)
	}

	return m, nil
}

func fetchOverviews(m *imap.Client, uids []int) error {
	fmt.Println("=== Fetching Overviews (Headers Only - FAST) ===")

	overviews, err := m.GetOverviews(ctx, uids...)
	if err != nil {
		return fmt.Errorf("failed to get overviews: %w", err)
	}

	for uid, email := range overviews {
		fmt.Printf("UID %d:\n", uid)
		fmt.Printf("  Subject: %s\n", email.Subject)
		fmt.Printf("  From: %s\n", email.From)
		fmt.Printf("  Date: %s\n", email.Sent)
		fmt.Printf("  Size: %d bytes (%.1f KB)\n", email.Size, float64(email.Size)/1024)
		fmt.Printf("  Flags: %v\n", email.Flags)
		fmt.Println()
	}

	return nil
}

func printEmailBody(email *imap.Email) {
	if len(email.Text) > 0 {
		preview := email.Text
		if len(preview) > 200 {
			preview = preview[:200] + "..."
		}
		// Clean up whitespace for display
		preview = strings.TrimSpace(preview)
		preview = strings.ReplaceAll(preview, "\n\n\n", "\n\n")
		fmt.Printf("\nText Preview:\n%s\n", preview)
		fmt.Printf("(Total text length: %d characters)\n", len(email.Text))
	}

	if len(email.HTML) > 0 {
		fmt.Printf("\nHTML content present: %d bytes (%.1f KB)\n",
			len(email.HTML), float64(len(email.HTML))/1024)
		htmlPreview := email.HTML
		if len(htmlPreview) > 100 {
			htmlPreview = htmlPreview[:100] + "..."
		}
		fmt.Printf("HTML Preview: %s\n", htmlPreview)
	}
}

func printAttachments(email *imap.Email) {
	if len(email.Attachments) > 0 {
		fmt.Printf("\nAttachments (%d):\n", len(email.Attachments))
		totalSize := 0
		for i, att := range email.Attachments {
			fmt.Printf("  %d. %s\n", i+1, att.Name)
			fmt.Printf("     - MIME Type: %s\n", att.MimeType)
			fmt.Printf("     - Size: %d bytes (%.1f KB)\n",
				len(att.Content), float64(len(att.Content))/1024)
			totalSize += len(att.Content)
		}
		fmt.Printf("  Total attachments size: %.1f KB\n", float64(totalSize)/1024)
	} else {
		fmt.Println("\nNo attachments")
	}
}

func fetchFullEmails(m *imap.Client, uids []int) (map[int]*imap.Email, error) {
	fmt.Println("=== Fetching Full Emails (With Bodies - SLOWER) ===")

	// Limit to first 3 for full fetch (to keep example fast)
	fetchUIDs := uids
	if len(uids) > 3 {
		fetchUIDs = uids[:3]
	}

	emails, err := m.GetEmails(ctx, fetchUIDs...)
	if err != nil {
		return nil, fmt.Errorf("failed to get emails: %w", err)
	}

	for uid, email := range emails {
		fmt.Printf("\n=== Email UID %d ===\n", uid)
		fmt.Printf("Subject: %s\n", email.Subject)
		fmt.Printf("From: %s\n", email.From)
		fmt.Printf("To: %s\n", email.To)
		fmt.Printf("CC: %s\n", email.CC)
		fmt.Printf("BCC: %s\n", email.BCC)
		fmt.Printf("Reply-To: %s\n", email.ReplyTo)
		fmt.Printf("Date Sent: %s\n", email.Sent)
		fmt.Printf("Date Received: %s\n", email.Received)
		fmt.Printf("Message-ID: %s\n", email.MessageID)
		fmt.Printf("Flags: %v\n", email.Flags)
		fmt.Printf("Size: %d bytes (%.1f KB)\n", email.Size, float64(email.Size)/1024)

		printEmailBody(email)
		printAttachments(email)

		fmt.Println("\n" + strings.Repeat("-", 50))
	}

	return emails, nil
}

func printEmailSummaries(emails map[int]*imap.Email) {
	fmt.Println("\n=== Using the String() Method ===")

	for uid, email := range emails {
		fmt.Printf("UID %d summary:\n", uid)
		fmt.Print(email)
		fmt.Println()
	}
}

func processAttachments(emails map[int]*imap.Email) {
	fmt.Println("\n=== Processing Attachments Example ===")

	for uid, email := range emails {
		if len(email.Attachments) > 0 {
			fmt.Printf("Email UID %d has %d attachment(s):\n", uid, len(email.Attachments))
			for _, att := range email.Attachments {
				fmt.Printf("  - %s: ", att.Name)

				// You could save attachments to disk like this:
				// err := os.WriteFile(att.Name, att.Content, 0644)
				// if err != nil {
				//     fmt.Printf("Failed to save: %v\n", err)
				// } else {
				//     fmt.Printf("Saved to disk\n")
				// }

				// For demo, just show what we would do
				fmt.Printf("Would save %d bytes to disk\n", len(att.Content))
			}
			break // Just show first email with attachments
		}
	}
}

func main() {
	m, err := connectAndSelectInbox()
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		if err := m.Close(); err != nil {
			log.Printf("Failed to close connection: %v", err)
		}
	}()

	uids, err := m.GetUIDs(ctx, "1:5") // Get first 5 emails
	if err != nil {
		log.Fatalf("Failed to get UIDs: %v", err)
	}

	if len(uids) == 0 {
		fmt.Println("No emails found in INBOX")
		return
	}

	fmt.Printf("Found %d emails to fetch\n\n", len(uids))

	if err := fetchOverviews(m, uids); err != nil {
		log.Fatal(err)
	}

	emails, err := fetchFullEmails(m, uids)
	if err != nil {
		log.Fatal(err)
	}

	printEmailSummaries(emails)
	processAttachments(emails)
}
