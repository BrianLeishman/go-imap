package imap

import (
	"testing"
	"time"
)

func TestSearchBuilder_Build(t *testing.T) {
	t.Parallel()

	// Fixed date for deterministic tests
	date := time.Date(2024, time.March, 15, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name     string
		builder  *SearchBuilder
		expected string
	}{
		{
			name:     "empty builder returns ALL",
			builder:  Search(),
			expected: "ALL",
		},
		{
			name:     "single flag",
			builder:  Search().Unseen(),
			expected: "UNSEEN",
		},
		{
			name:     "multiple flags",
			builder:  Search().Unseen().Flagged(),
			expected: "UNSEEN FLAGGED",
		},
		{
			name:     "all flag types",
			builder:  Search().Seen().Unseen().Flagged().Unflagged().Answered().Unanswered().Deleted().Undeleted().Draft().Undraft().Recent().New().Old(),
			expected: "SEEN UNSEEN FLAGGED UNFLAGGED ANSWERED UNANSWERED DELETED UNDELETED DRAFT UNDRAFT RECENT NEW OLD",
		},
		{
			name:     "from",
			builder:  Search().From("alice@example.com"),
			expected: `FROM "alice@example.com"`,
		},
		{
			name:     "to",
			builder:  Search().To("bob@example.com"),
			expected: `TO "bob@example.com"`,
		},
		{
			name:     "cc",
			builder:  Search().CC("carol@example.com"),
			expected: `CC "carol@example.com"`,
		},
		{
			name:     "bcc",
			builder:  Search().BCC("dave@example.com"),
			expected: `BCC "dave@example.com"`,
		},
		{
			name:     "subject",
			builder:  Search().Subject("meeting notes"),
			expected: `SUBJECT "meeting notes"`,
		},
		{
			name:     "body",
			builder:  Search().Body("invoice"),
			expected: `BODY "invoice"`,
		},
		{
			name:     "text",
			builder:  Search().Text("urgent"),
			expected: `TEXT "urgent"`,
		},
		{
			name:     "header",
			builder:  Search().Header("X-Mailer", "Outlook"),
			expected: `HEADER "X-Mailer" "Outlook"`,
		},
		{
			name:     "since",
			builder:  Search().Since(date),
			expected: "SINCE 15-Mar-2024",
		},
		{
			name:     "before",
			builder:  Search().Before(date),
			expected: "BEFORE 15-Mar-2024",
		},
		{
			name:     "on",
			builder:  Search().On(date),
			expected: "ON 15-Mar-2024",
		},
		{
			name:     "sent since",
			builder:  Search().SentSince(date),
			expected: "SENTSINCE 15-Mar-2024",
		},
		{
			name:     "sent before",
			builder:  Search().SentBefore(date),
			expected: "SENTBEFORE 15-Mar-2024",
		},
		{
			name:     "sent on",
			builder:  Search().SentOn(date),
			expected: "SENTON 15-Mar-2024",
		},
		{
			name:     "larger",
			builder:  Search().Larger(1024),
			expected: "LARGER 1024",
		},
		{
			name:     "smaller",
			builder:  Search().Smaller(5000000),
			expected: "SMALLER 5000000",
		},
		{
			name:     "uid set",
			builder:  Search().UID("1:100"),
			expected: "UID 1:100",
		},
		{
			name:     "keyword",
			builder:  Search().Keyword("$Forwarded"),
			expected: "KEYWORD $Forwarded",
		},
		{
			name:     "unkeyword",
			builder:  Search().Unkeyword("$Junk"),
			expected: "UNKEYWORD $Junk",
		},
		{
			name:     "not",
			builder:  Search().Not(Search().From("spam@example.com")),
			expected: `NOT (FROM "spam@example.com")`,
		},
		{
			name:     "or",
			builder:  Search().Or(Search().From("alice"), Search().From("bob")),
			expected: `OR (FROM "alice") (FROM "bob")`,
		},
		{
			name:     "complex query",
			builder:  Search().Unseen().From("alice@example.com").Since(date).Smaller(1000000),
			expected: `UNSEEN FROM "alice@example.com" SINCE 15-Mar-2024 SMALLER 1000000`,
		},
		{
			name:     "non-ASCII subject uses literal and charset",
			builder:  Search().Subject("тест"),
			expected: "CHARSET UTF-8 SUBJECT {8}\r\nтест",
		},
		{
			name:     "non-ASCII from uses literal and charset",
			builder:  Search().From("ユーザー"),
			expected: "CHARSET UTF-8 FROM {12}\r\nユーザー",
		},
		{
			name:     "mixed ASCII and non-ASCII",
			builder:  Search().From("alice@example.com").Subject("日報"),
			expected: `CHARSET UTF-8 FROM "alice@example.com" SUBJECT {6}` + "\r\n日報",
		},
		{
			name:     "or with non-ASCII propagates charset",
			builder:  Search().Or(Search().Subject("тест"), Search().Unseen()),
			expected: "CHARSET UTF-8 OR (CHARSET UTF-8 SUBJECT {8}\r\nтест) (UNSEEN)",
		},
		{
			name:     "all",
			builder:  Search().All(),
			expected: "ALL",
		},
		{
			name:     "header with non-ASCII value uses literal",
			builder:  Search().Header("X-Custom", "ünîcödé"),
			expected: `CHARSET UTF-8 HEADER "X-Custom" {11}` + "\r\nünîcödé",
		},
		{
			name:     "not with non-ASCII propagates charset",
			builder:  Search().Not(Search().Subject("日報")),
			expected: "CHARSET UTF-8 NOT (CHARSET UTF-8 SUBJECT {6}\r\n日報)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := tt.builder.Build()
			if got != tt.expected {
				t.Errorf("Build() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestNeedsLiteral(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input    string
		expected bool
	}{
		{"hello", false},
		{"alice@example.com", false},
		{"", false},
		{"тест", true},
		{"日本語", true},
		{"hello мир", true},
		{"café", true},
		{"Prüfung", true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			if got := needsLiteral(tt.input); got != tt.expected {
				t.Errorf("needsLiteral(%q) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}
