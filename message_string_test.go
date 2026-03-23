package imap

import (
	"mime"
	"strings"
	"testing"
	"time"
)

// --- EmailAddresses.String() ---

func TestEmailAddresses_String_Empty(t *testing.T) {
	t.Parallel()
	ea := EmailAddresses{}
	if s := ea.String(); s != "" {
		t.Errorf("expected empty string, got %q", s)
	}
}

func TestEmailAddresses_String_EmailOnly(t *testing.T) {
	t.Parallel()
	ea := EmailAddresses{"alice@example.com": ""}
	s := ea.String()
	if s != "alice@example.com" {
		t.Errorf("expected bare email, got %q", s)
	}
}

func TestEmailAddresses_String_WithName(t *testing.T) {
	t.Parallel()
	ea := EmailAddresses{"alice@example.com": "Alice"}
	s := ea.String()
	if !strings.Contains(s, "Alice") || !strings.Contains(s, "alice@example.com") {
		t.Errorf("expected name and email, got %q", s)
	}
}

func TestEmailAddresses_String_NameWithComma(t *testing.T) {
	t.Parallel()
	ea := EmailAddresses{"bob@example.com": "Doe, Bob"}
	s := ea.String()
	// Name with comma should be quoted
	if !strings.Contains(s, `"Doe, Bob"`) {
		t.Errorf("expected quoted name with comma, got %q", s)
	}
}

func TestEmailAddresses_String_Multiple(t *testing.T) {
	t.Parallel()
	ea := EmailAddresses{
		"alice@example.com": "Alice",
		"bob@example.com":   "Bob",
	}
	s := ea.String()
	if !strings.Contains(s, ", ") {
		t.Errorf("expected comma separator for multiple addresses, got %q", s)
	}
}

// --- Email.String() ---

func TestEmail_String_SubjectOnly(t *testing.T) {
	t.Parallel()
	e := Email{Subject: "Hello World"}
	s := e.String()
	if !strings.Contains(s, "Subject: Hello World") {
		t.Errorf("expected subject, got %q", s)
	}
}

func TestEmail_String_WithTo(t *testing.T) {
	t.Parallel()
	e := Email{
		Subject: "Test",
		To:      EmailAddresses{"alice@example.com": "Alice"},
	}
	s := e.String()
	if !strings.Contains(s, "To:") {
		t.Errorf("expected To field, got %q", s)
	}
}

func TestEmail_String_WithFrom(t *testing.T) {
	t.Parallel()
	e := Email{
		Subject: "Test",
		From:    EmailAddresses{"bob@example.com": "Bob"},
	}
	s := e.String()
	if !strings.Contains(s, "From:") {
		t.Errorf("expected From field, got %q", s)
	}
}

func TestEmail_String_WithCC(t *testing.T) {
	t.Parallel()
	e := Email{
		Subject: "Test",
		CC:      EmailAddresses{"cc@example.com": "CC"},
	}
	s := e.String()
	if !strings.Contains(s, "CC:") {
		t.Errorf("expected CC field, got %q", s)
	}
}

func TestEmail_String_WithBCC(t *testing.T) {
	t.Parallel()
	e := Email{
		Subject: "Test",
		BCC:     EmailAddresses{"bcc@example.com": "BCC"},
	}
	s := e.String()
	if !strings.Contains(s, "BCC:") {
		t.Errorf("expected BCC field, got %q", s)
	}
}

func TestEmail_String_WithReplyTo(t *testing.T) {
	t.Parallel()
	e := Email{
		Subject: "Test",
		ReplyTo: EmailAddresses{"reply@example.com": "Reply"},
	}
	s := e.String()
	if !strings.Contains(s, "ReplyTo:") {
		t.Errorf("expected ReplyTo field, got %q", s)
	}
}

func TestEmail_String_ShortText(t *testing.T) {
	t.Parallel()
	e := Email{Subject: "Test", Text: "short"}
	s := e.String()
	if !strings.Contains(s, "Text: short") {
		t.Errorf("expected short text, got %q", s)
	}
}

func TestEmail_String_LongText(t *testing.T) {
	t.Parallel()
	e := Email{Subject: "Test", Text: "this is a very long text body that exceeds twenty characters"}
	s := e.String()
	if !strings.Contains(s, "Text: this is a very long ") {
		t.Errorf("expected truncated text, got %q", s)
	}
	if !strings.Contains(s, "...") {
		t.Errorf("expected ellipsis in truncated text, got %q", s)
	}
}

func TestEmail_String_ShortHTML(t *testing.T) {
	t.Parallel()
	e := Email{Subject: "Test", HTML: "<b>hi</b>"}
	s := e.String()
	if !strings.Contains(s, "HTML: <b>hi</b>") {
		t.Errorf("expected short HTML, got %q", s)
	}
}

func TestEmail_String_LongHTML(t *testing.T) {
	t.Parallel()
	e := Email{Subject: "Test", HTML: "<html><body>this is a very long html content</body></html>"}
	s := e.String()
	// First 20 chars + "..." + size
	if !strings.Contains(s, "HTML:") || !strings.Contains(s, "...") {
		t.Errorf("expected truncated HTML with ellipsis, got %q", s)
	}
}

func TestEmail_String_WithAttachments(t *testing.T) {
	t.Parallel()
	e := Email{
		Subject: "Test",
		Attachments: []Attachment{
			{Name: "file.pdf", MimeType: "application/pdf", Content: []byte("data")},
		},
	}
	s := e.String()
	if !strings.Contains(s, "1 Attachment(s)") {
		t.Errorf("expected attachment info, got %q", s)
	}
}

func TestEmail_String_AllFields(t *testing.T) {
	t.Parallel()
	e := Email{
		Subject:     "Full Email",
		To:          EmailAddresses{"to@example.com": "To"},
		From:        EmailAddresses{"from@example.com": "From"},
		CC:          EmailAddresses{"cc@example.com": "CC"},
		BCC:         EmailAddresses{"bcc@example.com": "BCC"},
		ReplyTo:     EmailAddresses{"reply@example.com": "Reply"},
		Text:        "Hello there!",
		HTML:        "<p>Hello</p>",
		Attachments: []Attachment{{Name: "a.txt", MimeType: "text/plain", Content: []byte("x")}},
	}
	s := e.String()
	for _, field := range []string{"Subject:", "To:", "From:", "CC:", "BCC:", "ReplyTo:", "Text:", "HTML:", "Attachment"} {
		if !strings.Contains(s, field) {
			t.Errorf("expected %q in output, got %q", field, s)
		}
	}
}

// --- Attachment.String() ---

func TestAttachment_String(t *testing.T) {
	t.Parallel()
	a := Attachment{Name: "doc.pdf", MimeType: "application/pdf", Content: make([]byte, 1024)}
	s := a.String()
	if !strings.Contains(s, "doc.pdf") {
		t.Errorf("expected filename in string, got %q", s)
	}
	if !strings.Contains(s, "application/pdf") {
		t.Errorf("expected mime type in string, got %q", s)
	}
}

func TestAttachment_String_Empty(t *testing.T) {
	t.Parallel()
	a := Attachment{Name: "empty.txt", MimeType: "text/plain", Content: nil}
	s := a.String()
	if !strings.Contains(s, "empty.txt") {
		t.Errorf("expected filename, got %q", s)
	}
}

// --- unwrapTokens ---

func TestUnwrapTokens_SingleContainer(t *testing.T) {
	t.Parallel()
	inner := []*Token{
		{Type: TLiteral, Str: "UID"},
		{Type: TNumber, Num: 42},
	}
	wrapped := []*Token{{Type: TContainer, Tokens: inner}}
	result := unwrapTokens(wrapped)
	if len(result) != 2 {
		t.Fatalf("expected 2 tokens, got %d", len(result))
	}
	if result[0].Str != "UID" {
		t.Errorf("expected UID token, got %q", result[0].Str)
	}
}

func TestUnwrapTokens_DoubleContainer(t *testing.T) {
	t.Parallel()
	inner := []*Token{{Type: TLiteral, Str: "TEST"}}
	wrapped := []*Token{{Type: TContainer, Tokens: []*Token{
		{Type: TContainer, Tokens: inner},
	}}}
	result := unwrapTokens(wrapped)
	if len(result) != 1 || result[0].Str != "TEST" {
		t.Errorf("expected unwrapped TEST, got %+v", result)
	}
}

func TestUnwrapTokens_NoContainer(t *testing.T) {
	t.Parallel()
	tokens := []*Token{
		{Type: TLiteral, Str: "A"},
		{Type: TLiteral, Str: "B"},
	}
	result := unwrapTokens(tokens)
	if len(result) != 2 {
		t.Errorf("expected 2 tokens unchanged, got %d", len(result))
	}
}

func TestUnwrapTokens_Nil(t *testing.T) {
	t.Parallel()
	result := unwrapTokens(nil)
	if result != nil {
		t.Errorf("expected nil, got %v", result)
	}
}

// --- parseEmailBody ---

func TestParseEmailBody_SimpleMessage(t *testing.T) {
	t.Parallel()
	d := &Dialer{}
	e := &Email{}
	body := "From: sender@example.com\r\nTo: recipient@example.com\r\nSubject: Test Subject\r\nContent-Type: text/plain\r\n\r\nHello, World!"
	ok := d.parseEmailBody(e, body)
	if !ok {
		t.Fatal("expected parsing to succeed")
	}
	if e.Subject != "Test Subject" {
		t.Errorf("expected subject 'Test Subject', got %q", e.Subject)
	}
	if e.Text != "Hello, World!" {
		t.Errorf("expected text 'Hello, World!', got %q", e.Text)
	}
	if _, exists := e.From["sender@example.com"]; !exists {
		t.Error("expected From to contain sender@example.com")
	}
	if _, exists := e.To["recipient@example.com"]; !exists {
		t.Error("expected To to contain recipient@example.com")
	}
}

func TestParseEmailBody_HTMLMessage(t *testing.T) {
	t.Parallel()
	d := &Dialer{}
	e := &Email{}
	body := "From: a@b.com\r\nTo: c@d.com\r\nSubject: HTML\r\nContent-Type: text/html\r\n\r\n<html><body>Hello</body></html>"
	ok := d.parseEmailBody(e, body)
	if !ok {
		t.Fatal("expected parsing to succeed")
	}
	if e.HTML != "<html><body>Hello</body></html>" {
		t.Errorf("unexpected HTML: %q", e.HTML)
	}
}

func TestParseEmailBody_InvalidBody(t *testing.T) {
	t.Parallel()
	d := &Dialer{}
	e := &Email{}
	// Completely invalid content - enmime should still parse (it's lenient),
	// but let's test with something that might fail
	ok := d.parseEmailBody(e, "")
	// Empty body may or may not succeed depending on enmime
	_ = ok
}

// --- parseEmailRecord ---

func TestParseEmailRecord_BasicUID(t *testing.T) {
	t.Parallel()
	d := &Dialer{}
	tokens := []*Token{
		{Type: TLiteral, Str: "UID"},
		{Type: TNumber, Num: 42},
	}
	e, success, err := d.parseEmailRecord(tokens)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !success {
		t.Error("expected success")
	}
	if e.UID != 42 {
		t.Errorf("expected UID 42, got %d", e.UID)
	}
}

func TestParseEmailRecord_WithBody(t *testing.T) {
	t.Parallel()
	d := &Dialer{}
	body := "From: a@b.com\r\nSubject: Hi\r\nContent-Type: text/plain\r\n\r\nHello"
	tokens := []*Token{
		{Type: TLiteral, Str: "UID"},
		{Type: TNumber, Num: 10},
		{Type: TLiteral, Str: "BODY[]"},
		{Type: TAtom, Str: body},
	}
	e, success, err := d.parseEmailRecord(tokens)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !success {
		t.Error("expected success")
	}
	if e.UID != 10 {
		t.Errorf("expected UID 10, got %d", e.UID)
	}
	if e.Text != "Hello" {
		t.Errorf("expected text 'Hello', got %q", e.Text)
	}
}

func TestParseEmailRecord_WrappedInContainer(t *testing.T) {
	t.Parallel()
	d := &Dialer{}
	tokens := []*Token{{Type: TContainer, Tokens: []*Token{
		{Type: TLiteral, Str: "UID"},
		{Type: TNumber, Num: 99},
	}}}
	e, success, err := d.parseEmailRecord(tokens)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !success {
		t.Error("expected success")
	}
	if e.UID != 99 {
		t.Errorf("expected UID 99, got %d", e.UID)
	}
}

// --- parseEnvelope ---

func TestParseEnvelope_Basic(t *testing.T) {
	t.Parallel()
	d := &Dialer{}
	e := &Email{}
	envelope := &Token{Type: TContainer, Tokens: []*Token{
		{Type: TQuoted, Str: "Mon, 23 Mar 2026 10:00:00 -0500"}, // date
		{Type: TQuoted, Str: "Test Subject"},                      // subject
		{Type: TContainer, Tokens: []*Token{ // from
			{Type: TContainer, Tokens: []*Token{
				{Type: TQuoted, Str: "Alice"},
				{Type: TNil},
				{Type: TQuoted, Str: "alice"},
				{Type: TQuoted, Str: "example.com"},
			}},
		}},
		{Type: TNil}, // sender
		{Type: TContainer, Tokens: []*Token{ // reply-to
			{Type: TContainer, Tokens: []*Token{
				{Type: TQuoted, Str: "Alice"},
				{Type: TNil},
				{Type: TQuoted, Str: "alice"},
				{Type: TQuoted, Str: "example.com"},
			}},
		}},
		{Type: TContainer, Tokens: []*Token{ // to
			{Type: TContainer, Tokens: []*Token{
				{Type: TQuoted, Str: "Bob"},
				{Type: TNil},
				{Type: TQuoted, Str: "bob"},
				{Type: TQuoted, Str: "example.com"},
			}},
		}},
		{Type: TNil}, // cc
		{Type: TNil}, // bcc
		{Type: TNil}, // in-reply-to
		{Type: TQuoted, Str: "<msg123@example.com>"}, // message-id
	}}

	tks := []*Token{
		{Type: TLiteral, Str: "ENVELOPE"},
		envelope,
	}
	err := d.parseEnvelope(e, envelope, tks)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if e.Subject != "Test Subject" {
		t.Errorf("expected 'Test Subject', got %q", e.Subject)
	}
	if _, ok := e.From["alice@example.com"]; !ok {
		t.Error("expected From to contain alice@example.com")
	}
	if _, ok := e.To["bob@example.com"]; !ok {
		t.Error("expected To to contain bob@example.com")
	}
	if e.MessageID != "<msg123@example.com>" {
		t.Errorf("expected message ID, got %q", e.MessageID)
	}
}

func TestParseEnvelope_NilSubject(t *testing.T) {
	t.Parallel()
	d := &Dialer{}
	e := &Email{}
	envelope := &Token{Type: TContainer, Tokens: []*Token{
		{Type: TQuoted, Str: "Mon, 23 Mar 2026 10:00:00 -0500"},
		{Type: TNil}, // nil subject
		{Type: TNil},
		{Type: TNil},
		{Type: TNil},
		{Type: TNil},
		{Type: TNil},
		{Type: TNil},
		{Type: TNil},
		{Type: TNil},
	}}
	err := d.parseEnvelope(e, envelope, []*Token{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if e.Subject != "" {
		t.Errorf("expected empty subject, got %q", e.Subject)
	}
}

// --- parseOverviewField ---

func TestParseOverviewField_FLAGS(t *testing.T) {
	t.Parallel()
	d := &Dialer{}
	e := &Email{}
	tks := []*Token{
		{Type: TLiteral, Str: "FLAGS"},
		{Type: TContainer, Tokens: []*Token{
			{Type: TLiteral, Str: `\Seen`},
			{Type: TLiteral, Str: `\Flagged`},
		}},
	}
	skip, err := d.parseOverviewField(e, tks, 0, "FLAGS")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if skip != 1 {
		t.Errorf("expected skip=1, got %d", skip)
	}
	if len(e.Flags) != 2 {
		t.Fatalf("expected 2 flags, got %d", len(e.Flags))
	}
	if e.Flags[0] != `\Seen` {
		t.Errorf("expected \\Seen, got %q", e.Flags[0])
	}
}

func TestParseOverviewField_INTERNALDATE(t *testing.T) {
	t.Parallel()
	d := &Dialer{}
	e := &Email{}
	tks := []*Token{
		{Type: TLiteral, Str: "INTERNALDATE"},
		{Type: TQuoted, Str: "23-Mar-2026 15:04:05 +0000"},
	}
	skip, err := d.parseOverviewField(e, tks, 0, "INTERNALDATE")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if skip != 1 {
		t.Errorf("expected skip=1, got %d", skip)
	}
	if e.Received.IsZero() {
		t.Error("expected non-zero Received time")
	}
}

func TestParseOverviewField_RFC822SIZE(t *testing.T) {
	t.Parallel()
	d := &Dialer{}
	e := &Email{}
	tks := []*Token{
		{Type: TLiteral, Str: "RFC822.SIZE"},
		{Type: TNumber, Num: 12345},
	}
	skip, err := d.parseOverviewField(e, tks, 0, "RFC822.SIZE")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if skip != 1 {
		t.Errorf("expected skip=1, got %d", skip)
	}
	if e.Size != 12345 {
		t.Errorf("expected size 12345, got %d", e.Size)
	}
}

func TestParseOverviewField_UID(t *testing.T) {
	t.Parallel()
	d := &Dialer{}
	e := &Email{}
	tks := []*Token{
		{Type: TLiteral, Str: "UID"},
		{Type: TNumber, Num: 789},
	}
	skip, err := d.parseOverviewField(e, tks, 0, "UID")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if skip != 1 {
		t.Errorf("expected skip=1, got %d", skip)
	}
	if e.UID != 789 {
		t.Errorf("expected UID 789, got %d", e.UID)
	}
}

func TestParseOverviewField_ENVELOPE(t *testing.T) {
	t.Parallel()
	d := &Dialer{}
	e := &Email{}
	envelope := &Token{Type: TContainer, Tokens: []*Token{
		{Type: TQuoted, Str: "Mon, 23 Mar 2026 10:00:00 -0500"},
		{Type: TQuoted, Str: "Subject Here"},
		{Type: TNil},
		{Type: TNil},
		{Type: TNil},
		{Type: TNil},
		{Type: TNil},
		{Type: TNil},
		{Type: TNil},
		{Type: TQuoted, Str: "<id@example.com>"},
	}}
	tks := []*Token{
		{Type: TLiteral, Str: "ENVELOPE"},
		envelope,
	}
	skip, err := d.parseOverviewField(e, tks, 0, "ENVELOPE")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if skip != 1 {
		t.Errorf("expected skip=1, got %d", skip)
	}
	if e.Subject != "Subject Here" {
		t.Errorf("expected 'Subject Here', got %q", e.Subject)
	}
}

func TestParseOverviewField_Unknown(t *testing.T) {
	t.Parallel()
	d := &Dialer{}
	e := &Email{}
	tks := []*Token{
		{Type: TLiteral, Str: "UNKNOWN"},
	}
	skip, err := d.parseOverviewField(e, tks, 0, "UNKNOWN")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if skip != 0 {
		t.Errorf("expected skip=0 for unknown field, got %d", skip)
	}
}

// --- parseOverviewRecord ---

func TestParseOverviewRecord_Full(t *testing.T) {
	t.Parallel()
	d := &Dialer{}
	envelope := &Token{Type: TContainer, Tokens: []*Token{
		{Type: TQuoted, Str: "Mon, 23 Mar 2026 10:00:00 -0500"},
		{Type: TQuoted, Str: "Full Overview"},
		{Type: TNil},
		{Type: TNil},
		{Type: TNil},
		{Type: TNil},
		{Type: TNil},
		{Type: TNil},
		{Type: TNil},
		{Type: TQuoted, Str: "<id@test.com>"},
	}}
	tks := []*Token{
		{Type: TLiteral, Str: "FLAGS"},
		{Type: TContainer, Tokens: []*Token{
			{Type: TLiteral, Str: `\Seen`},
		}},
		{Type: TLiteral, Str: "INTERNALDATE"},
		{Type: TQuoted, Str: "23-Mar-2026 15:04:05 +0000"},
		{Type: TLiteral, Str: "RFC822.SIZE"},
		{Type: TNumber, Num: 5000},
		{Type: TLiteral, Str: "ENVELOPE"},
		envelope,
		{Type: TLiteral, Str: "UID"},
		{Type: TNumber, Num: 100},
	}
	e, err := d.parseOverviewRecord(tks)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if e.UID != 100 {
		t.Errorf("expected UID 100, got %d", e.UID)
	}
	if e.Size != 5000 {
		t.Errorf("expected size 5000, got %d", e.Size)
	}
	if e.Subject != "Full Overview" {
		t.Errorf("expected 'Full Overview', got %q", e.Subject)
	}
	if len(e.Flags) != 1 {
		t.Errorf("expected 1 flag, got %d", len(e.Flags))
	}
}

func TestParseOverviewRecord_WrappedContainer(t *testing.T) {
	t.Parallel()
	d := &Dialer{}
	tks := []*Token{{Type: TContainer, Tokens: []*Token{
		{Type: TLiteral, Str: "UID"},
		{Type: TNumber, Num: 55},
	}}}
	e, err := d.parseOverviewRecord(tks)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if e.UID != 55 {
		t.Errorf("expected UID 55, got %d", e.UID)
	}
}

// --- parseEnvelopeAddresses ---

func TestParseEnvelopeAddresses_NilToken(t *testing.T) {
	t.Parallel()
	d := &Dialer{}
	dest := &EmailAddresses{}
	err := d.parseEnvelopeAddresses(dest, &Token{Type: TNil}, nil, nil, "TEST")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseEnvelopeAddresses_MultipleAddresses(t *testing.T) {
	t.Parallel()
	d := &Dialer{}
	dest := &EmailAddresses{}
	dec := &mime.WordDecoder{}
	addrToken := &Token{Type: TContainer, Tokens: []*Token{
		{Type: TContainer, Tokens: []*Token{
			{Type: TQuoted, Str: "Alice"},
			{Type: TNil},
			{Type: TQuoted, Str: "alice"},
			{Type: TQuoted, Str: "example.com"},
		}},
		{Type: TContainer, Tokens: []*Token{
			{Type: TQuoted, Str: "Bob"},
			{Type: TNil},
			{Type: TQuoted, Str: "bob"},
			{Type: TQuoted, Str: "example.com"},
		}},
	}}
	err := d.parseEnvelopeAddresses(dest, addrToken, dec, nil, "TEST")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(*dest) != 2 {
		t.Errorf("expected 2 addresses, got %d", len(*dest))
	}
	if (*dest)["alice@example.com"] != "Alice" {
		t.Errorf("expected Alice, got %q", (*dest)["alice@example.com"])
	}
	if (*dest)["bob@example.com"] != "Bob" {
		t.Errorf("expected Bob, got %q", (*dest)["bob@example.com"])
	}
}

// --- parseSingleFetchLine ---

func TestParseSingleFetchLine_Basic(t *testing.T) {
	t.Parallel()
	tokens, err := parseSingleFetchLine("* 1 FETCH (UID 42)\r\n")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tokens) < 2 {
		t.Fatalf("expected at least 2 tokens, got %d", len(tokens))
	}
}

func TestParseSingleFetchLine_InvalidPrefix(t *testing.T) {
	t.Parallel()
	_, err := parseSingleFetchLine("bad line")
	if err == nil {
		t.Fatal("expected error for invalid prefix")
	}
}

func TestParseSingleFetchLine_NoSeqNum(t *testing.T) {
	t.Parallel()
	_, err := parseSingleFetchLine("* ")
	if err == nil {
		t.Fatal("expected error for missing seq num")
	}
}

func TestParseSingleFetchLine_InvalidSeqNum(t *testing.T) {
	t.Parallel()
	_, err := parseSingleFetchLine("* abc FETCH (UID 1)\r\n")
	if err == nil {
		t.Fatal("expected error for invalid seq num")
	}
}

func TestParseSingleFetchLine_NoFetchKeyword(t *testing.T) {
	t.Parallel()
	_, err := parseSingleFetchLine("* 1 NOTFETCH (UID 1)\r\n")
	if err == nil {
		t.Fatal("expected error for missing FETCH keyword")
	}
}

// --- TimeFormat constant test ---

func TestTimeFormatParsing(t *testing.T) {
	t.Parallel()
	ts := "23-Mar-2026 15:04:05 +0000"
	parsed, err := time.Parse(TimeFormat, ts)
	if err != nil {
		t.Fatalf("unexpected error parsing TimeFormat: %v", err)
	}
	if parsed.Year() != 2026 {
		t.Errorf("expected year 2026, got %d", parsed.Year())
	}
}
