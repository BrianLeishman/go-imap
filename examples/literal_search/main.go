package main

import (
	"fmt"
)

func main() {
	fmt.Println("=== RFC 3501 Section 7.5 Literal Search Example ===")
	fmt.Println("This example demonstrates searching with non-ASCII characters using literal syntax")
	fmt.Println()

	// Example usage with literal syntax for UTF-8 characters
	examples := []struct {
		description string
		searchQuery string
	}{
		{
			"Search for Cyrillic text in subject",
			`CHARSET UTF-8 Subject {8}` + "\r\n" + "тест",
		},
		{
			"Search for Chinese text in subject",
			`CHARSET UTF-8 Subject {6}` + "\r\n" + "测试",
		},
		{
			"Search for Japanese text in subject",
			`CHARSET UTF-8 Subject {6}` + "\r\n" + "テスト",
		},
		{
			"Search for Arabic text in subject",
			`CHARSET UTF-8 Subject {8}` + "\r\n" + "اختبار",
		},
		{
			"Search for emoji in body",
			`CHARSET UTF-8 BODY {4}` + "\r\n" + "😀👍",
		},
	}

	fmt.Println("Example search queries with literal syntax:")
	for i, example := range examples {
		fmt.Printf("\n%d. %s:\n", i+1, example.description)
		fmt.Printf("   Query: %q\n", example.searchQuery)

		// Show the literal detection
		if containsLiteralDemo(example.searchQuery) {
			fmt.Printf("   ✅ Literal syntax detected - will use ExecWithLiteral\n")
		} else {
			fmt.Printf("   ℹ️  No literal syntax - will use regular Exec\n")
		}
	}

	fmt.Println()
	fmt.Println("To use this in your code:")
	fmt.Printf(`
// Connect to your IMAP server
im, err := imap.New("username", "password", "mail.server.com", 993)
if err != nil {
    log.Fatal(err)
}
defer im.Close()

// Select folder
err = im.SelectFolder("INBOX")
if err != nil {
    log.Fatal(err)
}

// Search with literal syntax for non-ASCII characters
uids, err := im.GetUIDs("CHARSET UTF-8 Subject {8}\r\nтест")
if err != nil {
    log.Fatal(err)
}

fmt.Printf("Found %%d emails matching Cyrillic 'тест'\n", len(uids))
`)

	fmt.Println("\nNote: The literal syntax {n} specifies the number of bytes in the following data.")
	fmt.Println("This is crucial for non-ASCII characters which may use multiple bytes per character.")
}

// Simple demonstration function to show literal detection logic
func containsLiteralDemo(command string) bool {
	// Simple regex match for demo purposes
	for i := 0; i < len(command)-1; i++ {
		if command[i] == '{' {
			for j := i + 1; j < len(command); j++ {
				if command[j] == '}' {
					// Check if what's between braces is a number
					numStr := command[i+1 : j]
					if len(numStr) > 0 {
						// Simple digit check
						allDigits := true
						for _, c := range numStr {
							if c < '0' || c > '9' {
								allDigits = false
								break
							}
						}
						if allDigits {
							return true
						}
					}
					break
				}
			}
		}
	}
	return false
}
