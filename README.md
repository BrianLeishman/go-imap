# Simple IMAP Client Library

I wasn't able to find an IMAP client I liked (or found easy to use), so, now there's also this one. My goal here is to allow people to download emails quickly, and robustly, that's it.

## Getting Started

```shell
go get github.com/BrianLeishman/go-imap
```

## Usage

Below I've written a super basic demo function of what this library is capable of doing, and how one might use it.

```go
package main

import (
	"fmt"

	"github.com/BrianLeishman/go-imap"
)

func check(err error) {
	if err != nil {
		panic(err)
	}
}

func main() {

	// Defaults to false. This package level option turns on or off debugging output, essentially.
	// If verbose is set to true, then every command, and every response, is printed,
	// along with other things like error messages (before the retry limit is reached)
	imap.Verbose = true

	// Defaults to 10. Certain functions retry; like the login function, and the new connection function.
	// If a retried function fails, the connection will be closed, then the program sleeps for an increasing amount of time,
	// creates a new connection instance internally, selects the same folder, and retries the failed command(s).
	// You can check out github.com/StirlingMarketingGroup/go-retry for the retry implementation being used
	imap.RetryCount = 3

	// Create a new instance of the IMAP connection you want to use
	im, err := imap.New("username", "password", "mail.server.com", 993)
	check(err)
	defer im.Close()

	// Folders now contains a string slice of all the folder names on the connection
	folders, err := im.GetFolders()
	check(err)

	// folders = []string{
	// 	"INBOX",
	// 	"INBOX/My Folder"
	// 	"Sent Items",
	// 	"Deleted",
	// }

	// Now we can loop through those folders
	for _, f := range folders {

		// And select each folder, one at a time.
		// Whichever folder is selected last, is the current active folder.
		// All following commands will be executing inside of this folder
		err = im.SelectFolder(f)
		check(err)

		// This function implements the IMAP UID search, returning a slice of ints
		// Sending "ALL" runs the command "UID SEARCH ALL"
		// You can enter things like "*:1" to get the first UID, or "999999999:*"
		// to get the last (unless you actually have more than that many emails)
		// You can check out https://tools.ietf.org/html/rfc3501#section-6.4.4 for more
		uids, err := im.GetUIDs("ALL")
		check(err)

		// uids = []int{1, 2, 3}

		// GetEmails takes a list of ints as UIDs, and returns new Email objects.
		// If an email for a given UID cannot be found, there's an error parsing its body,
		// or the email addresses are malformed (like, missing parts of the address), then it is skipped
		// If an email is found, then an imap.Email struct slice is returned with the information from the email.
		// The Email struct looks like this:
		// type Email struct {
		// 	Flags     []string
		// 	Received  time.Time
		// 	Sent      time.Time
		// 	Size      uint64
		// 	Subject   string
		// 	UID       int
		// 	MessageID string
		// 	From      EmailAddresses
		// 	To        EmailAddresses
		// 	ReplyTo   EmailAddresses
		// 	CC        EmailAddresses
		// 	BCC       EmailAddresses
		// 	Text      string
		// 	HTML      string
		//	Attachments []Attachment
		// }
		// Where the address type fields are maps like [EmailAddress:Name EmailAddress2:Name2]
		// and an Attachment is a struct containing the Name and the Content (both as strings)
		emails, err := im.GetEmails(uids...)
		check(err)

		if len(emails) != 0 {
			// Should print a summary of one of the the emails
			// (yes, I said "one of", don't expect the emails to be returned in any particular order)
			fmt.Print(emails[0])
			// Subject: FW: FW:  FW:  New Order
			// To: Brian Leishman <brian@stumpyinc.com>
			// From: Customer Service <sales@totallylegitdomain.com>
			// Text: Hello, World!...(4.3 kB)
			// HTML: <html xmlns:v="urn:s... (35 kB)
			// 1 Attachment(s): [20180330174029.jpg (192 kB)]
		}

	}

}
```

## Built With

- [jhillyerd/enmime](github.com/jhillyerd/enmime) - MIME mail encoding and decoding library for Go
- [logrusorgru/aurora](github.com/logrusorgru/aurora) - Golang ultimate ANSI-colors that supports Printf/Sprintf methods

## Authors

- **Brian Leishman** - [Stirling Marketing Group](https://github.com/StirlingMarketingGroup)
