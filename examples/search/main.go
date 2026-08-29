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
	// Connect to server
	m, err := imap.Dial(context.Background(), imap.Options{
		Host: "mail.server.com",
		Port: 993,
		Auth: imap.PasswordAuth{Username: "username", Password: "password"},
	})
	if err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}
	defer func() {
		if err := m.Close(); err != nil {
			log.Printf("Failed to close connection: %v", err)
		}
	}()

	err = m.SelectFolder(ctx, "INBOX")
	if err != nil {
		log.Fatalf("Failed to select INBOX: %v", err)
	}

	fmt.Println("=== Basic Searches ===")

	// Basic searches - returns slice of UIDs
	allUIDs, _ := m.GetUIDs(ctx, "ALL")         // All emails
	unseenUIDs, _ := m.GetUIDs(ctx, "UNSEEN")   // Unread emails
	recentUIDs, _ := m.GetUIDs(ctx, "RECENT")   // Recent emails
	seenUIDs, _ := m.GetUIDs(ctx, "SEEN")       // Read emails
	flaggedUIDs, _ := m.GetUIDs(ctx, "FLAGGED") // Starred/flagged emails

	fmt.Printf("Found %d total emails\n", len(allUIDs))
	fmt.Printf("Found %d unread emails\n", len(unseenUIDs))
	fmt.Printf("Found %d recent emails\n", len(recentUIDs))
	fmt.Printf("Found %d read emails\n", len(seenUIDs))
	fmt.Printf("Found %d flagged emails\n", len(flaggedUIDs))

	if len(unseenUIDs) > 0 && len(unseenUIDs) <= 10 {
		fmt.Printf("UIDs of unread: %v\n", unseenUIDs)
	}

	fmt.Println("\n=== Date-based Searches ===")

	// Date-based searches
	// Note: Use RFC 2822 date format
	today := time.Now().Format("2-Jan-2006")
	weekAgo := time.Now().AddDate(0, 0, -7).Format("2-Jan-2006")
	monthAgo := time.Now().AddDate(0, -1, 0).Format("2-Jan-2006")

	todayUIDs, _ := m.GetUIDs(ctx, fmt.Sprintf("ON %s", today))
	sinceUIDs, _ := m.GetUIDs(ctx, fmt.Sprintf("SINCE %s", weekAgo))
	beforeUIDs, _ := m.GetUIDs(ctx, fmt.Sprintf("BEFORE %s", today))
	rangeUIDs, _ := m.GetUIDs(ctx, fmt.Sprintf("SINCE %s BEFORE %s", monthAgo, today))

	fmt.Printf("Emails from today: %d\n", len(todayUIDs))
	fmt.Printf("Emails since a week ago: %d\n", len(sinceUIDs))
	fmt.Printf("Emails before today: %d\n", len(beforeUIDs))
	fmt.Printf("Emails in the last month: %d\n", len(rangeUIDs))

	fmt.Println("\n=== Sender/Recipient Searches ===")

	// From/To searches
	fromBossUIDs, _ := m.GetUIDs(ctx, `FROM "boss@company.com"`)
	toMeUIDs, _ := m.GetUIDs(ctx, `TO "me@company.com"`)
	ccUIDs, _ := m.GetUIDs(ctx, `CC "team@company.com"`)

	fmt.Printf("Emails from boss: %d\n", len(fromBossUIDs))
	fmt.Printf("Emails to me: %d\n", len(toMeUIDs))
	fmt.Printf("Emails CC'd to team: %d\n", len(ccUIDs))

	fmt.Println("\n=== Content Searches ===")

	// Subject/body searches
	subjectUIDs, _ := m.GetUIDs(ctx, `SUBJECT "invoice"`)
	bodyUIDs, _ := m.GetUIDs(ctx, `BODY "payment"`)
	textUIDs, _ := m.GetUIDs(ctx, `TEXT "urgent"`) // Searches both subject and body

	fmt.Printf("Emails with 'invoice' in subject: %d\n", len(subjectUIDs))
	fmt.Printf("Emails with 'payment' in body: %d\n", len(bodyUIDs))
	fmt.Printf("Emails with 'urgent' anywhere: %d\n", len(textUIDs))

	fmt.Println("\n=== Complex Searches ===")

	complexUIDs1, _ := m.GetUIDs(ctx, `UNSEEN FROM "support@github.com" SINCE 1-Jan-2024`)
	complexUIDs2, _ := m.GetUIDs(ctx, `FLAGGED SUBJECT "important" SINCE 1-Jan-2024`)
	complexUIDs3, _ := m.GetUIDs(ctx, `NOT SEEN NOT FROM "noreply@" SINCE 1-Jan-2024`)

	fmt.Printf("Unread emails from GitHub support this year: %d\n", len(complexUIDs1))
	fmt.Printf("Flagged emails with 'important' in subject this year: %d\n", len(complexUIDs2))
	fmt.Printf("Unread emails not from noreply addresses this year: %d\n", len(complexUIDs3))

	fmt.Println("\n=== UID Ranges ===")

	firstUID, _ := m.GetUIDs(ctx, "1")       // First email
	lastUID, _ := m.GetUIDs(ctx, "*")        // Last email
	maxUID, err := m.GetMaxUID(ctx)        // Last email (cheaper, requires RFC-4731)
	first10UIDs, _ := m.GetUIDs(ctx, "1:10") // First 10 emails
	last10UIDs, _ := m.GetUIDs(ctx, "*:10")  // Last 10 emails (reverse)

	fmt.Printf("First email UID: %v\n", firstUID)
	fmt.Printf("Last email UID: %v\n", lastUID)
	if err != nil {
		fmt.Printf("Last email UID (alt): not supported (%v)\n", err)
	} else {
		fmt.Printf("Last email UID (alt): %v\n", maxUID)
	}
	fmt.Printf("First 10 email UIDs: %v\n", first10UIDs)
	if len(last10UIDs) <= 10 {
		fmt.Printf("Last 10 email UIDs: %v\n", last10UIDs)
	} else {
		fmt.Printf("Last 10 email UIDs: %d emails found\n", len(last10UIDs))
	}

	fmt.Println("\n=== Size-based Searches ===")

	// Size-based searches (in bytes)
	largeUIDs, _ := m.GetUIDs(ctx, "LARGER 10485760") // Emails larger than 10MB
	mediumUIDs, _ := m.GetUIDs(ctx, "LARGER 1048576") // Emails larger than 1MB
	smallUIDs, _ := m.GetUIDs(ctx, "SMALLER 10240")   // Emails smaller than 10KB

	fmt.Printf("Emails larger than 10MB: %d\n", len(largeUIDs))
	fmt.Printf("Emails larger than 1MB: %d\n", len(mediumUIDs))
	fmt.Printf("Emails smaller than 10KB: %d\n", len(smallUIDs))

	fmt.Println("\n=== Search Builder (Type-Safe API) ===")

	// The SearchBuilder provides a fluent, type-safe alternative to raw strings
	unseenBuilderUIDs, _ := m.SearchUIDs(ctx, imap.Search().Unseen())
	fmt.Printf("Unread emails (builder): %d\n", len(unseenBuilderUIDs))

	// Combine multiple criteria
	weekAgoTime := time.Now().AddDate(0, 0, -7)
	recentUnread, _ := m.SearchUIDs(ctx, 
		imap.Search().Unseen().Since(weekAgoTime),
	)
	fmt.Printf("Unread emails from last 7 days (builder): %d\n", len(recentUnread))

	// OR: messages from either sender
	fromEither, _ := m.SearchUIDs(ctx, 
		imap.Search().Or(
			imap.Search().From("boss@company.com"),
			imap.Search().From("support@github.com"),
		),
	)
	fmt.Printf("Emails from boss or GitHub support: %d\n", len(fromEither))

	// NOT: exclude certain senders
	notNoreply, _ := m.SearchUIDs(ctx, 
		imap.Search().Not(imap.Search().From("noreply@")).Unseen(),
	)
	fmt.Printf("Unread emails NOT from noreply: %d\n", len(notNoreply))

	// Size filter
	largeBuilder, _ := m.SearchUIDs(ctx, imap.Search().Larger(1048576))
	fmt.Printf("Emails larger than 1MB (builder): %d\n", len(largeBuilder))

	// You can also get the raw string from a builder
	query := imap.Search().From("boss@company.com").Unseen().Since(weekAgoTime).Build()
	fmt.Printf("Built query string: %s\n", query)

	fmt.Println("\n=== Special Searches ===")

	answeredUIDs, _ := m.GetUIDs(ctx, "ANSWERED")
	unansweredUIDs, _ := m.GetUIDs(ctx, "UNANSWERED")
	deletedUIDs, _ := m.GetUIDs(ctx, "DELETED")
	undeletedUIDs, _ := m.GetUIDs(ctx, "UNDELETED")
	draftUIDs, _ := m.GetUIDs(ctx, "DRAFT")
	undraftUIDs, _ := m.GetUIDs(ctx, "UNDRAFT")

	fmt.Printf("Answered emails: %d\n", len(answeredUIDs))
	fmt.Printf("Unanswered emails: %d\n", len(unansweredUIDs))
	fmt.Printf("Deleted emails: %d\n", len(deletedUIDs))
	fmt.Printf("Not deleted emails: %d\n", len(undeletedUIDs))
	fmt.Printf("Draft emails: %d\n", len(draftUIDs))
	fmt.Printf("Non-draft emails: %d\n", len(undraftUIDs))
}
