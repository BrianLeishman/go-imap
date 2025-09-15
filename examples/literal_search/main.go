package main

import (
	"fmt"
	"log"
	"strings"

	imap "github.com/BrianLeishman/go-imap"
)

func main() {
	fmt.Println("=== RFC 3501 Section 7.5 Literal Search Example ===")
	fmt.Println("This example demonstrates searching with non-ASCII characters using literal syntax")
	fmt.Println()

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

	// Select folder first
	err = m.SelectFolder("INBOX")
	if err != nil {
		log.Fatalf("Failed to select INBOX: %v", err)
	}

	fmt.Println("Connected and selected INBOX successfully!")
	fmt.Println()

	// Example searches with literal syntax for various character sets
	// Using the new MakeIMAPLiteral helper function for convenience
	literalSearches := []struct {
		description string
		query       string
		language    string
	}{
		{
			"Search for Cyrillic text 'тест' (test) in subject",
			`CHARSET UTF-8 Subject ` + imap.MakeIMAPLiteral("тест"),
			"Russian",
		},
		{
			"Search for Chinese text '测试' (test) in subject",
			`CHARSET UTF-8 Subject ` + imap.MakeIMAPLiteral("测试"),
			"Chinese",
		},
		{
			"Search for Japanese text 'テスト' (test) in subject",
			`CHARSET UTF-8 Subject ` + imap.MakeIMAPLiteral("テスト"),
			"Japanese",
		},
		{
			"Search for Arabic text 'اختبار' (test) in subject",
			`CHARSET UTF-8 Subject ` + imap.MakeIMAPLiteral("اختبار"),
			"Arabic",
		},
		{
			"Search for emoji '😀👍' in body text",
			`CHARSET UTF-8 BODY ` + imap.MakeIMAPLiteral("😀👍"),
			"Emoji",
		},
		{
			"Search for German umlaut 'Prüfung' (test) in subject",
			`CHARSET UTF-8 Subject ` + imap.MakeIMAPLiteral("Prüfung"),
			"German",
		},
	}

	fmt.Println("=== Non-ASCII Searches Using Literal Syntax ===")
	fmt.Println()

	for i, search := range literalSearches {
		fmt.Printf("%d. %s (%s)\n", i+1, search.description, search.language)
		fmt.Printf("   Query: CHARSET UTF-8 Subject/BODY {n}\\r\\n<text>\n")
		fmt.Printf("   Actual bytes: %d\n", len([]byte(search.query[strings.LastIndex(search.query, "\n")+1:])))

		// Perform the search
		uids, err := m.GetUIDs(search.query)
		if err != nil {
			fmt.Printf("   ❌ Search failed: %v\n", err)
		} else if len(uids) == 0 {
			fmt.Printf("   ℹ️  No emails found matching this criteria\n")
		} else {
			fmt.Printf("   ✅ Found %d email(s) with UIDs: %v\n", len(uids), uids[:min(len(uids), 5)])
			if len(uids) > 5 {
				fmt.Printf("   ... and %d more\n", len(uids)-5)
			}
		}
		fmt.Println()
	}

	// Compare with regular ASCII search
	fmt.Println("=== Regular ASCII Search (for comparison) ===")
	asciiSearches := []string{
		`SUBJECT "test"`,
		`BODY "hello"`,
		`FROM "example.com"`,
	}

	for _, search := range asciiSearches {
		fmt.Printf("Query: %s\n", search)
		uids, err := m.GetUIDs(search)
		if err != nil {
			fmt.Printf("❌ Search failed: %v\n", err)
		} else {
			fmt.Printf("✅ Found %d email(s)\n", len(uids))
		}
		fmt.Println()
	}

	fmt.Println("=== Key Points About Literal Syntax ===")
	fmt.Println("• Use CHARSET UTF-8 for non-ASCII searches")
	fmt.Println("• The {n} syntax specifies exact byte count (not character count!)")
	fmt.Println("• UTF-8 characters may use 1-4 bytes per character")
	fmt.Println("• The library automatically detects {n} syntax and handles the continuation protocol")
	fmt.Println("• Backward compatibility: regular ASCII searches work unchanged")
	fmt.Println("• NEW: Use imap.MakeIMAPLiteral() helper for automatic byte counting")
	fmt.Println()

	fmt.Println("=== MakeIMAPLiteral Helper Function Examples ===")
	helperExamples := []string{"test", "тест", "测试", "😀👍"}
	for _, text := range helperExamples {
		literal := imap.MakeIMAPLiteral(text)
		fmt.Printf("imap.MakeIMAPLiteral(\"%s\") = \"%s\"\n", text, strings.ReplaceAll(literal, "\r\n", "\\r\\n"))
	}
	fmt.Println()

	fmt.Println("Example byte counts for different characters:")
	examples := map[string]string{
		"test":    "4 bytes (ASCII)",
		"тест":    "8 bytes (Cyrillic)",
		"测试":      "6 bytes (Chinese)",
		"テスト":     "9 bytes (Japanese)",
		"اختبار":  "12 bytes (Arabic)",
		"😀👍":      "8 bytes (Emoji)",
		"Prüfung": "8 bytes (German with umlaut)",
	}

	for text, info := range examples {
		fmt.Printf("• '%s' = %s\n", text, info)
	}
}

// Helper function for Go versions without min builtin
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
