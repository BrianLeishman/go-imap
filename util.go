package imap

import "fmt"

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
