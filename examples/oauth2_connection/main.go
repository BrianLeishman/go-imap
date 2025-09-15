package main

import (
	"fmt"
	"log"
	"time"

	imap "github.com/BrianLeishman/go-imap"
)

func main() {
	imap.DialTimeout = 10 * time.Second
	imap.CommandTimeout = 30 * time.Second

	// OAuth2 access token - you need to obtain this from your OAuth2 flow
	// For Gmail: https://developers.google.com/gmail/api/auth/about-auth
	// For Office 365: https://docs.microsoft.com/en-us/azure/active-directory/develop/v2-oauth2-auth-code-flow
	accessToken := "your-oauth2-access-token"

	// Connect with OAuth2 (Gmail example)
	m, err := imap.NewWithOAuth2("user@example.com", accessToken, "imap.gmail.com", 993)
	if err != nil {
		log.Fatalf("Failed to connect with OAuth2: %v", err)
	}
	defer func() {
		if err := m.Close(); err != nil {
			log.Printf("Failed to close connection: %v", err)
		}
	}()

	// The OAuth2 connection works exactly like LOGIN after authentication
	if err := m.SelectFolder("INBOX"); err != nil {
		log.Fatalf("Failed to select INBOX: %v", err)
	}

	unreadUIDs, err := m.GetUIDs("UNSEEN")
	if err != nil {
		log.Fatalf("Failed to search for unread emails: %v", err)
	}

	fmt.Printf("Connected via OAuth2!\n")
	fmt.Printf("You have %d unread emails in INBOX\n", len(unreadUIDs))
}
