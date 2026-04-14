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
	// OAuth2 access token - you need to obtain this from your OAuth2 flow
	// For Gmail: https://developers.google.com/gmail/api/auth/about-auth
	// For Office 365: https://docs.microsoft.com/en-us/azure/active-directory/develop/v2-oauth2-auth-code-flow
	accessToken := "your-oauth2-access-token"

	// Connect with OAuth2 (Gmail example)
	m, err := imap.Dial(context.Background(), imap.Options{
		Host:           "imap.gmail.com",
		Port:           993,
		Auth:           imap.XOAuth2{Username: "user@example.com", AccessToken: accessToken},
		DialTimeout:    10 * time.Second,
		CommandTimeout: 30 * time.Second,
	})
	if err != nil {
		log.Fatalf("Failed to connect with OAuth2: %v", err)
	}
	defer func() {
		if err := m.Close(); err != nil {
			log.Printf("Failed to close connection: %v", err)
		}
	}()

	// The OAuth2 connection works exactly like LOGIN after authentication
	if err := m.SelectFolder(ctx, "INBOX"); err != nil {
		log.Fatalf("Failed to select INBOX: %v", err)
	}

	unreadUIDs, err := m.GetUIDs(ctx, "UNSEEN")
	if err != nil {
		log.Fatalf("Failed to search for unread emails: %v", err)
	}

	fmt.Printf("Connected via OAuth2!\n")
	fmt.Printf("You have %d unread emails in INBOX\n", len(unreadUIDs))
}
