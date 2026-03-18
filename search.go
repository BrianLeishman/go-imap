package imap

import (
	"fmt"
	"strings"
	"time"
)

// imapDateFormat is the date format used in IMAP SEARCH commands (RFC 3501 §6.4.4).
const imapDateFormat = "2-Jan-2006"

// SearchBuilder constructs IMAP SEARCH criteria using a fluent builder pattern.
// All criteria are AND-ed together (IMAP implicit conjunction).
//
// Example:
//
//	uids, err := conn.SearchUIDs(
//	    imap.Search().From("alice@example.com").Unseen().Since(lastWeek),
//	)
type SearchBuilder struct {
	criteria     []string
	needsCharset bool
}

// Search returns a new SearchBuilder.
func Search() *SearchBuilder {
	return &SearchBuilder{}
}

// Build returns the assembled IMAP SEARCH criteria string.
// If no criteria have been added, it returns "ALL".
// CHARSET UTF-8 is prepended when any criterion contains non-ASCII text.
func (s *SearchBuilder) Build() string {
	raw := s.buildRaw()
	if s.needsCharset {
		return "CHARSET UTF-8 " + raw
	}
	return raw
}

// buildRaw returns criteria without the CHARSET prefix.
// Used by Not/Or to avoid emitting CHARSET inside nested search keys.
func (s *SearchBuilder) buildRaw() string {
	if len(s.criteria) == 0 {
		return "ALL"
	}
	return strings.Join(s.criteria, " ")
}

// SearchUIDs executes a search using the builder and returns matching UIDs.
//
// Example:
//
//	uids, err := conn.SearchUIDs(imap.Search().From("alice@example.com").Unseen())
func (d *Dialer) SearchUIDs(search *SearchBuilder) ([]int, error) {
	return d.GetUIDs(search.Build())
}

// --- Flag criteria ---

// All matches all messages.
func (s *SearchBuilder) All() *SearchBuilder { s.criteria = append(s.criteria, "ALL"); return s }

// Seen matches messages with the \Seen flag.
func (s *SearchBuilder) Seen() *SearchBuilder { s.criteria = append(s.criteria, "SEEN"); return s }

// Unseen matches messages without the \Seen flag.
func (s *SearchBuilder) Unseen() *SearchBuilder {
	s.criteria = append(s.criteria, "UNSEEN")
	return s
}

// Flagged matches messages with the \Flagged flag.
func (s *SearchBuilder) Flagged() *SearchBuilder {
	s.criteria = append(s.criteria, "FLAGGED")
	return s
}

// Unflagged matches messages without the \Flagged flag.
func (s *SearchBuilder) Unflagged() *SearchBuilder {
	s.criteria = append(s.criteria, "UNFLAGGED")
	return s
}

// Answered matches messages with the \Answered flag.
func (s *SearchBuilder) Answered() *SearchBuilder {
	s.criteria = append(s.criteria, "ANSWERED")
	return s
}

// Unanswered matches messages without the \Answered flag.
func (s *SearchBuilder) Unanswered() *SearchBuilder {
	s.criteria = append(s.criteria, "UNANSWERED")
	return s
}

// Deleted matches messages with the \Deleted flag.
func (s *SearchBuilder) Deleted() *SearchBuilder {
	s.criteria = append(s.criteria, "DELETED")
	return s
}

// Undeleted matches messages without the \Deleted flag.
func (s *SearchBuilder) Undeleted() *SearchBuilder {
	s.criteria = append(s.criteria, "UNDELETED")
	return s
}

// Draft matches messages with the \Draft flag.
func (s *SearchBuilder) Draft() *SearchBuilder {
	s.criteria = append(s.criteria, "DRAFT")
	return s
}

// Undraft matches messages without the \Draft flag.
func (s *SearchBuilder) Undraft() *SearchBuilder {
	s.criteria = append(s.criteria, "UNDRAFT")
	return s
}

// Recent matches messages with the \Recent flag.
func (s *SearchBuilder) Recent() *SearchBuilder {
	s.criteria = append(s.criteria, "RECENT")
	return s
}

// New matches messages that are both \Recent and not \Seen.
func (s *SearchBuilder) New() *SearchBuilder { s.criteria = append(s.criteria, "NEW"); return s }

// Old matches messages without the \Recent flag.
func (s *SearchBuilder) Old() *SearchBuilder { s.criteria = append(s.criteria, "OLD"); return s }

// Keyword matches messages with the specified keyword flag.
func (s *SearchBuilder) Keyword(keyword string) *SearchBuilder {
	s.criteria = append(s.criteria, "KEYWORD "+keyword)
	return s
}

// Unkeyword matches messages without the specified keyword flag.
func (s *SearchBuilder) Unkeyword(keyword string) *SearchBuilder {
	s.criteria = append(s.criteria, "UNKEYWORD "+keyword)
	return s
}

// --- Header/content criteria ---

func (s *SearchBuilder) addTextCriteria(key, value string) {
	if needsLiteral(value) {
		s.needsCharset = true
		s.criteria = append(s.criteria, key+" "+MakeIMAPLiteral(value))
	} else {
		s.criteria = append(s.criteria, fmt.Sprintf("%s %q", key, value))
	}
}

// needsLiteral returns true if the string contains non-ASCII characters
// and must be sent using IMAP literal syntax.
func needsLiteral(s string) bool {
	for _, r := range s {
		if r > 127 {
			return true
		}
	}
	return false
}

// From matches messages with the specified string in the From header.
func (s *SearchBuilder) From(addr string) *SearchBuilder {
	s.addTextCriteria("FROM", addr)
	return s
}

// To matches messages with the specified string in the To header.
func (s *SearchBuilder) To(addr string) *SearchBuilder {
	s.addTextCriteria("TO", addr)
	return s
}

// CC matches messages with the specified string in the CC header.
func (s *SearchBuilder) CC(addr string) *SearchBuilder {
	s.addTextCriteria("CC", addr)
	return s
}

// BCC matches messages with the specified string in the BCC header.
func (s *SearchBuilder) BCC(addr string) *SearchBuilder {
	s.addTextCriteria("BCC", addr)
	return s
}

// Subject matches messages with the specified string in the Subject header.
func (s *SearchBuilder) Subject(subject string) *SearchBuilder {
	s.addTextCriteria("SUBJECT", subject)
	return s
}

// Body matches messages with the specified string in the message body.
func (s *SearchBuilder) Body(text string) *SearchBuilder {
	s.addTextCriteria("BODY", text)
	return s
}

// Text matches messages with the specified string in the header or body.
func (s *SearchBuilder) Text(text string) *SearchBuilder {
	s.addTextCriteria("TEXT", text)
	return s
}

// Header matches messages with the specified value in the named header field.
func (s *SearchBuilder) Header(field, value string) *SearchBuilder {
	if needsLiteral(value) {
		s.needsCharset = true
		s.criteria = append(s.criteria, fmt.Sprintf("HEADER %q %s", field, MakeIMAPLiteral(value)))
	} else {
		s.criteria = append(s.criteria, fmt.Sprintf("HEADER %q %q", field, value))
	}
	return s
}

// --- Date criteria ---

// Since matches messages with an internal date on or after the given date.
func (s *SearchBuilder) Since(t time.Time) *SearchBuilder {
	s.criteria = append(s.criteria, "SINCE "+t.Format(imapDateFormat))
	return s
}

// Before matches messages with an internal date before the given date.
func (s *SearchBuilder) Before(t time.Time) *SearchBuilder {
	s.criteria = append(s.criteria, "BEFORE "+t.Format(imapDateFormat))
	return s
}

// On matches messages with an internal date equal to the given date.
func (s *SearchBuilder) On(t time.Time) *SearchBuilder {
	s.criteria = append(s.criteria, "ON "+t.Format(imapDateFormat))
	return s
}

// SentSince matches messages with a Date header on or after the given date.
func (s *SearchBuilder) SentSince(t time.Time) *SearchBuilder {
	s.criteria = append(s.criteria, "SENTSINCE "+t.Format(imapDateFormat))
	return s
}

// SentBefore matches messages with a Date header before the given date.
func (s *SearchBuilder) SentBefore(t time.Time) *SearchBuilder {
	s.criteria = append(s.criteria, "SENTBEFORE "+t.Format(imapDateFormat))
	return s
}

// SentOn matches messages with a Date header equal to the given date.
func (s *SearchBuilder) SentOn(t time.Time) *SearchBuilder {
	s.criteria = append(s.criteria, "SENTON "+t.Format(imapDateFormat))
	return s
}

// --- Size criteria ---

// Larger matches messages with a size (in bytes) greater than the specified value.
func (s *SearchBuilder) Larger(bytes int) *SearchBuilder {
	s.criteria = append(s.criteria, fmt.Sprintf("LARGER %d", bytes))
	return s
}

// Smaller matches messages with a size (in bytes) less than the specified value.
func (s *SearchBuilder) Smaller(bytes int) *SearchBuilder {
	s.criteria = append(s.criteria, fmt.Sprintf("SMALLER %d", bytes))
	return s
}

// --- UID criteria ---

// UID matches messages in the specified UID set (e.g., "1:100", "5,10,15").
func (s *SearchBuilder) UID(set string) *SearchBuilder {
	s.criteria = append(s.criteria, "UID "+set)
	return s
}

// --- Logical operators ---

// Not negates the given search criteria.
//
// Example:
//
//	// Messages NOT from alice
//	Search().Not(Search().From("alice@example.com"))
func (s *SearchBuilder) Not(inner *SearchBuilder) *SearchBuilder {
	s.criteria = append(s.criteria, "NOT ("+inner.buildRaw()+")")
	if inner.needsCharset {
		s.needsCharset = true
	}
	return s
}

// Or matches messages that satisfy either set of criteria.
// IMAP OR takes exactly two search keys.
//
// Example:
//
//	// Messages from alice OR bob
//	Search().Or(Search().From("alice"), Search().From("bob"))
func (s *SearchBuilder) Or(a, b *SearchBuilder) *SearchBuilder {
	s.criteria = append(s.criteria, fmt.Sprintf("OR (%s) (%s)", a.buildRaw(), b.buildRaw()))
	if a.needsCharset || b.needsCharset {
		s.needsCharset = true
	}
	return s
}
