package imap

import (
	"fmt"
	"time"

	"github.com/logrusorgru/aurora"
)

// log outputs a formatted message with timestamp and connection info
func log(connNum int, folder string, msg interface{}) {
	var name string
	if len(folder) != 0 {
		name = fmt.Sprintf("IMAP%d:%s", connNum, folder)
	} else {
		name = fmt.Sprintf("IMAP%d", connNum)
	}
	fmt.Println(aurora.Sprintf("%s %s: %s", time.Now().Format("2006-01-02 15:04:05.000000"), aurora.Colorize(name, aurora.CyanFg|aurora.BoldFm), msg))
}

// dropNl removes trailing newline characters from a byte slice
func dropNl(b []byte) []byte {
	if len(b) >= 1 && b[len(b)-1] == '\n' {
		if len(b) >= 2 && b[len(b)-2] == '\r' {
			return b[:len(b)-2]
		} else {
			return b[:len(b)-1]
		}
	}
	return b
}

// MakeIMAPLiteral generates IMAP literal syntax for non-ASCII strings.
// It returns a string in the format "{bytecount}\r\ntext" where bytecount
// is the number of bytes (not characters) in the input string.
// This is useful for search queries with non-ASCII characters.
// Example: MakeIMAPLiteral("тест") returns "{8}\r\nтест"
func MakeIMAPLiteral(s string) string {
	return fmt.Sprintf("{%d}\r\n%s", len([]byte(s)), s)
}
