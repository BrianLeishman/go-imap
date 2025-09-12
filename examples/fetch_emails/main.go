package main

import (
	"fmt"
	"log"
	"strings"

	imap "github.com/BrianLeishman/go-imap"
)

func main() {
	// Connect to server
	m, err := imap.New("username", "password", "mail.server.com", 993)
	if err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}
	defer m.Close()

	// Select folder
	err = m.SelectFolder("INBOX")
	if err != nil {
		log.Fatalf("Failed to select INBOX: %v", err)
	}

	// Get some UIDs to work with
	uids, err := m.GetUIDs("1:5") // Get first 5 emails
	if err != nil {
		log.Fatalf("Failed to get UIDs: %v", err)
	}

	if len(uids) == 0 {
		fmt.Println("No emails found in INBOX")
		return
	}

	fmt.Printf("Found %d emails to fetch\n\n", len(uids))

	fmt.Println("=== Fetching Overviews (Headers Only - FAST) ===")

	// Get overview (headers only, no body) - FAST
	overviews, err := m.GetOverviews(uids...)
	if err != nil {
		log.Fatalf("Failed to get overviews: %v", err)
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

	fmt.Println("=== Fetching Full Emails (With Bodies - SLOWER) ===")

	// Limit to first 3 for full fetch (to keep example fast)
	fetchUIDs := uids
	if len(uids) > 3 {
		fetchUIDs = uids[:3]
	}

	// Get full emails with bodies - SLOWER
	emails, err := m.GetEmails(fetchUIDs...)
	if err != nil {
		log.Fatalf("Failed to get emails: %v", err)
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

		// Body content
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
			// Show first 100 chars of HTML
			htmlPreview := email.HTML
			if len(htmlPreview) > 100 {
				htmlPreview = htmlPreview[:100] + "..."
			}
			fmt.Printf("HTML Preview: %s\n", htmlPreview)
		}

		// Attachments
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

		fmt.Println("\n" + strings.Repeat("-", 50))
	}

	fmt.Println("\n=== Using the String() Method ===")

	// The String() method provides a quick summary
	for uid, email := range emails {
		fmt.Printf("UID %d summary:\n", uid)
		fmt.Print(email)
		fmt.Println()
	}

	// Example of processing attachments
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
