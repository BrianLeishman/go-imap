package imap

import (
	"context"
	"fmt"
	"io"
	"math"
	"mime"
	"reflect"
	"strconv"
	"strings"
	"time"

	retry "github.com/StirlingMarketingGroup/go-retry"
	"github.com/davecgh/go-spew/spew"
	humanize "github.com/dustin/go-humanize"
	"github.com/jhillyerd/enmime/v2"
	"golang.org/x/net/html/charset"
)

// uidFromToken validates and converts a parsed TNumber into a UID, rejecting
// values outside RFC 3501's 32-bit unsigned range so a malformed server
// response cannot silently wrap.
func uidFromToken(n int64) (UID, error) {
	if n < 0 || n > math.MaxUint32 {
		return 0, fmt.Errorf("UID %d out of 32-bit range", n)
	}
	return UID(n), nil
}

// UID is an IMAP unique identifier (RFC 3501 §2.3.1.1). UIDs are 32-bit
// unsigned integers scoped to a mailbox + UIDVALIDITY value.
type UID uint32

// MessageSeq is an IMAP message sequence number (RFC 3501 §2.3.1.2). Sequence
// numbers are 1-based positions within the currently selected mailbox and
// change as messages are added or expunged — prefer UIDs for durable
// references. Zero is not a valid sequence number.
type MessageSeq uint32

// String returns the UID formatted as a decimal string, suitable for
// embedding in IMAP command arguments.
func (u UID) String() string { return strconv.FormatUint(uint64(u), 10) }

// String returns the sequence number formatted as a decimal string, suitable
// for embedding in IMAP command arguments.
func (s MessageSeq) String() string { return strconv.FormatUint(uint64(s), 10) }

// EmailAddresses represents a map of email addresses to display names
type EmailAddresses map[string]string

// Email represents an IMAP email message
type Email struct {
	Flags       []string
	Received    time.Time
	Sent        time.Time
	Size        uint64
	Subject     string
	UID         UID
	MessageID   string
	From        EmailAddresses
	To          EmailAddresses
	ReplyTo     EmailAddresses
	CC          EmailAddresses
	BCC         EmailAddresses
	Text        string
	HTML        string
	Attachments []Attachment
}

// Attachment represents an email attachment
type Attachment struct {
	Name     string
	MimeType string
	Content  []byte
}

// Email parsing constants
const (
	EDate uint8 = iota
	ESubject
	EFrom
	ESender
	EReplyTo
	ETo
	ECC
	EBCC
	EInReplyTo
	EMessageID
)

const (
	EEName uint8 = iota
	EESR
	EEMailbox
	EEHost
)

// String returns a formatted string representation of EmailAddresses
func (e EmailAddresses) String() string {
	emails := strings.Builder{}
	i := 0
	for e, n := range e {
		if i != 0 {
			emails.WriteString(", ")
		}
		if len(n) != 0 {
			if strings.ContainsRune(n, ',') {
				fmt.Fprintf(&emails, `"%s" <%s>`, AddSlashes.Replace(n), e)
			} else {
				fmt.Fprintf(&emails, `%s <%s>`, n, e)
			}
		} else {
			emails.WriteString(e)
		}
		i++
	}
	return emails.String()
}

// String returns a formatted string representation of an Email
func (e Email) String() string {
	email := strings.Builder{}

	fmt.Fprintf(&email, "Subject: %s\n", e.Subject)

	if len(e.To) != 0 {
		fmt.Fprintf(&email, "To: %s\n", e.To)
	}
	if len(e.From) != 0 {
		fmt.Fprintf(&email, "From: %s\n", e.From)
	}
	if len(e.CC) != 0 {
		fmt.Fprintf(&email, "CC: %s\n", e.CC)
	}
	if len(e.BCC) != 0 {
		fmt.Fprintf(&email, "BCC: %s\n", e.BCC)
	}
	if len(e.ReplyTo) != 0 {
		fmt.Fprintf(&email, "ReplyTo: %s\n", e.ReplyTo)
	}
	if len(e.Text) != 0 {
		if len(e.Text) > 20 {
			fmt.Fprintf(&email, "Text: %s...", e.Text[:20])
		} else {
			fmt.Fprintf(&email, "Text: %s", e.Text)
		}
		fmt.Fprintf(&email, "(%s)\n", humanize.Bytes(uint64(len(e.Text))))
	}
	if len(e.HTML) != 0 {
		if len(e.HTML) > 20 {
			fmt.Fprintf(&email, "HTML: %s...", e.HTML[:20])
		} else {
			fmt.Fprintf(&email, "HTML: %s", e.HTML)
		}
		fmt.Fprintf(&email, " (%s)\n", humanize.Bytes(uint64(len(e.HTML))))
	}

	if len(e.Attachments) != 0 {
		fmt.Fprintf(&email, "%d Attachment(s): %s\n", len(e.Attachments), e.Attachments)
	}

	return email.String()
}

// String returns a formatted string representation of an Attachment
func (a Attachment) String() string {
	return fmt.Sprintf("%s (%s %s)", a.Name, a.MimeType, humanize.Bytes(uint64(len(a.Content))))
}

// GetUIDs retrieves message UIDs matching a search criteria.
// The search parameter is passed directly to the IMAP UID SEARCH command.
// See RFC 3501 for search criteria syntax.
//
// Common examples:
//   - "ALL" - all messages
//   - "UNSEEN" - unread messages
//   - "SEEN" - read messages
//   - "1:10" - UIDs 1 through 10
//   - "SINCE 1-Jan-2024" - messages since a date
//
// Note: For retrieving the N most recent messages, use GetLastNUIDs instead.
func (d *Client) GetUIDs(ctx context.Context, search string) (uids []UID, err error) {
	r, err := d.Exec(ctx, `UID SEARCH `+search, true, d.effectiveRetryCount(), nil)
	if err != nil {
		return nil, err
	}
	return parseUIDSearchResponse(r)
}

// GetLastNUIDs returns the N messages with the highest UIDs in the selected folder.
// This is useful for fetching the most recent messages.
//
// Note: This method fetches all UIDs from the server and returns the last N.
// For mailboxes with many thousands of messages, consider using GetUIDs with
// a specific UID range if you know the approximate UID values you need.
//
// Example:
//
//	// Get the 10 most recent messages
//	uids, err := conn.GetLastNUIDs(10)
func (d *Client) GetLastNUIDs(ctx context.Context, n int) ([]UID, error) {
	if n <= 0 {
		return nil, nil
	}
	allUIDs, err := d.GetUIDs(ctx, "ALL")
	if err != nil {
		return nil, err
	}
	if len(allUIDs) <= n {
		return allUIDs, nil
	}
	return allUIDs[len(allUIDs)-n:], nil
}

// Get max UID in the current folder using RFC-4731.
//
// The folder of interest must be already selected in either read-only mode,
// ExamineFolder, or in read-write mode, SelectFolder.
func (d *Client) GetMaxUID(ctx context.Context) (uid UID, err error) {
	r, err := d.Exec(ctx, "UID SEARCH RETURN (MAX) 1:*", true, d.effectiveRetryCount(), nil)
	if err != nil {
		return 0, err
	}
	return parseMaxUIDSearchResponse(r)
}

// restoreReadOnly re-EXAMINEs the current folder to restore read-only mode
// after a temporary SELECT. The caller's ctx cancellation is detached so a
// cancelled or timed-out mutation cannot leave the client stuck in
// read-write mode; cleanupContext bounds the EXAMINE so a stalled server
// cannot hang the call.
func (d *Client) restoreReadOnly(ctx context.Context) error {
	restoreCtx, cancel := cleanupContext(ctx)
	defer cancel()
	return d.ExamineFolder(restoreCtx, d.Folder)
}

// MoveEmail moves an email to a different folder.
func (d *Client) MoveEmail(ctx context.Context, uid UID, folder string) (err error) {
	// if we are currently read-only, switch to SELECT for the move-operation
	readOnlyState := d.ReadOnly
	if readOnlyState {
		_ = d.SelectFolder(ctx, d.Folder)
	}
	_, err = d.Exec(ctx, `UID MOVE `+uid.String()+` "`+AddSlashes.Replace(folder)+`"`, true, d.effectiveRetryCount(), nil)
	if readOnlyState {
		_ = d.restoreReadOnly(ctx)
	}
	if err != nil {
		return err
	}
	d.Folder = folder
	return nil
}

// CopyEmail copies an email to a different folder.
// Unlike MoveEmail, the original message remains in the current folder.
// UID COPY is not retried because duplicating a message is not idempotent.
func (d *Client) CopyEmail(ctx context.Context, uid UID, folder string) error {
	readOnlyState := d.ReadOnly
	if readOnlyState {
		if err := d.SelectFolder(ctx, d.Folder); err != nil {
			return err
		}
	}
	_, err := d.Exec(ctx, `UID COPY `+uid.String()+` "`+AddSlashes.Replace(folder)+`"`, true, 0, nil)
	if readOnlyState {
		if e := d.restoreReadOnly(ctx); e != nil && err == nil {
			err = e
		}
	}
	return err
}

// MarkSeen marks an email as seen/read.
func (d *Client) MarkSeen(ctx context.Context, uid UID) (err error) {
	flags := Flags{
		Seen: FlagAdd,
	}

	readOnlyState := d.ReadOnly
	if readOnlyState {
		_ = d.SelectFolder(ctx, d.Folder)
	}
	err = d.SetFlags(ctx, uid, flags)
	if readOnlyState {
		_ = d.restoreReadOnly(ctx)
	}

	return err
}

// DeleteEmail marks an email for deletion.
func (d *Client) DeleteEmail(ctx context.Context, uid UID) (err error) {
	flags := Flags{
		Deleted: FlagAdd,
	}

	readOnlyState := d.ReadOnly
	if readOnlyState {
		if err = d.SelectFolder(ctx, d.Folder); err != nil {
			return err
		}
	}
	err = d.SetFlags(ctx, uid, flags)
	if readOnlyState {
		if e := d.restoreReadOnly(ctx); e != nil && err == nil {
			err = e
		}
	}

	return err
}

// Expunge permanently removes emails marked for deletion.
func (d *Client) Expunge(ctx context.Context) (err error) {
	readOnlyState := d.ReadOnly
	if readOnlyState {
		if err = d.SelectFolder(ctx, d.Folder); err != nil {
			return err
		}
	}
	_, err = d.Exec(ctx, "EXPUNGE", false, d.effectiveRetryCount(), nil)
	if readOnlyState {
		if e := d.restoreReadOnly(ctx); e != nil && err == nil {
			err = e
		}
	}
	return err
}

// SetFlags sets message flags (seen, deleted, etc.).
func (d *Client) SetFlags(ctx context.Context, uid UID, flags Flags) (err error) {
	// craft the flags-string
	addFlags := []string{}
	removeFlags := []string{}

	v := reflect.ValueOf(flags)
	t := reflect.TypeOf(flags)

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		value := v.Field(i)

		if field.Type == reflect.TypeOf(FlagUnset) {
			switch FlagSet(value.Int()) {
			case FlagAdd:
				addFlags = append(addFlags, `\`+field.Name)
			case FlagRemove:
				removeFlags = append(removeFlags, `\`+field.Name)
			}
		}
	}

	// iterate over the keyword-map and add those too to the slices
	for keyword, state := range flags.Keywords {
		if state {
			addFlags = append(addFlags, keyword)
		} else {
			removeFlags = append(removeFlags, keyword)
		}
	}

	query := fmt.Sprintf("UID STORE %d", uid)
	if len(addFlags) > 0 {
		query += fmt.Sprintf(` +FLAGS (%s)`, strings.Join(addFlags, " "))
	}
	if len(removeFlags) > 0 {
		query += fmt.Sprintf(` -FLAGS (%s)`, strings.Join(removeFlags, " "))
	}

	// if we are currently read-only, switch to SELECT for the move-operation
	readOnlyState := d.ReadOnly
	if readOnlyState {
		_ = d.SelectFolder(ctx, d.Folder)
	}
	_, err = d.Exec(ctx, query, true, d.effectiveRetryCount(), nil)
	if readOnlyState {
		_ = d.restoreReadOnly(ctx)
	}

	return err
}

// parseEmailBody parses an RFC 2822 message body string and populates the Email fields.
// Returns true if parsing succeeded.
func (d *Client) parseEmailBody(e *Email, bodyStr string) bool {
	r := strings.NewReader(bodyStr)
	env, err := enmime.ReadEnvelope(r)
	if err != nil {
		if Verbose {
			warnLog(d.ConnNum, d.Folder, "email body could not be parsed", "error", err)
			spew.Dump(env)
			spew.Dump(bodyStr)
		}
		return false
	}

	e.Subject = env.GetHeader("Subject")
	e.Text = env.Text
	e.HTML = env.HTML

	for _, a := range env.Attachments {
		e.Attachments = append(e.Attachments, Attachment{
			Name:     a.FileName,
			MimeType: a.ContentType,
			Content:  a.Content,
		})
	}
	for _, a := range env.Inlines {
		e.Attachments = append(e.Attachments, Attachment{
			Name:     a.FileName,
			MimeType: a.ContentType,
			Content:  a.Content,
		})
	}

	for _, a := range []struct {
		dest   *EmailAddresses
		header string
	}{
		{&e.From, "From"},
		{&e.ReplyTo, "Reply-To"},
		{&e.To, "To"},
		{&e.CC, "cc"},
		{&e.BCC, "bcc"},
	} {
		alist, _ := env.AddressList(a.header)
		(*a.dest) = make(map[string]string, len(alist))
		for _, addr := range alist {
			(*a.dest)[strings.ToLower(addr.Address)] = addr.Name
		}
	}
	return true
}

// unwrapTokens flattens single-child TContainer wrappers that some servers add.
func unwrapTokens(tks []*Token) []*Token {
	for len(tks) == 1 && tks[0].Type == TContainer {
		tks = tks[0].Tokens
	}
	return tks
}

// parseEmailRecord processes a single FETCH record for GetEmails, returning the
// parsed email, whether parsing succeeded, and any error.
func (d *Client) parseEmailRecord(tks []*Token) (*Email, bool, error) {
	tks = unwrapTokens(tks)
	e := &Email{}
	skip := 0
	success := true
	for i, t := range tks {
		if skip > 0 {
			skip--
			continue
		}
		if err := d.CheckType(t, []TType{TLiteral}, tks, "in root"); err != nil {
			return nil, false, err
		}
		switch t.Str {
		case "BODY[]":
			if err := d.CheckType(tks[i+1], []TType{TAtom}, tks, "after BODY[]"); err != nil {
				return nil, false, err
			}
			if !d.parseEmailBody(e, tks[i+1].Str) {
				success = false
			}
			skip++
		case "UID":
			if err := d.CheckType(tks[i+1], []TType{TNumber}, tks, "after UID"); err != nil {
				return nil, false, err
			}
			u, err := uidFromToken(tks[i+1].Num)
			if err != nil {
				return nil, false, err
			}
			e.UID = u
			skip++
		}
	}
	return e, success, nil
}

// GetEmails retrieves full email messages including body content.
func (d *Client) GetEmails(ctx context.Context, uids ...UID) (emails map[UID]*Email, err error) {
	emails, err = d.GetOverviews(ctx, uids...)
	if err != nil {
		return nil, err
	}

	if len(emails) == 0 {
		return emails, err
	}

	uidsStr := strings.Builder{}
	if len(uids) == 0 {
		uidsStr.WriteString("1:*")
	} else {
		i := 0
		for u := range emails {
			if u == 0 {
				continue
			}

			if i != 0 {
				uidsStr.WriteByte(',')
			}
			uidsStr.WriteString(u.String())
			i++
		}
	}

	var records [][]*Token
	err = retry.Retry(func() (err error) {
		r, err := d.Exec(ctx, "UID FETCH "+uidsStr.String()+" BODY.PEEK[]", true, 0, nil)
		if err != nil {
			return err
		}

		records, err = d.ParseFetchResponse(r)
		if err != nil {
			return err
		}

		for _, tks := range records {
			e, success, err := d.parseEmailRecord(tks)
			if err != nil {
				return err
			}
			if success {
				if emails[e.UID] == nil {
					emails[e.UID] = &Email{UID: e.UID}
				}
				emails[e.UID].Subject = e.Subject
				emails[e.UID].From = e.From
				emails[e.UID].ReplyTo = e.ReplyTo
				emails[e.UID].To = e.To
				emails[e.UID].CC = e.CC
				emails[e.UID].BCC = e.BCC
				emails[e.UID].Text = e.Text
				emails[e.UID].HTML = e.HTML
				emails[e.UID].Attachments = e.Attachments
			} else {
				delete(emails, e.UID)
			}
		}
		return err
	}, d.effectiveRetryCount(), func(err error) error {
		errorLog(d.ConnNum, d.Folder, "fetch failed", "error", err)
		_ = d.Close()
		return nil
	}, func() error {
		return d.Reconnect(ctx)
	})

	return emails, err
}

// parseEnvelope extracts envelope data (date, subject, addresses, message-id) from an ENVELOPE token.
func (d *Client) parseEnvelope(e *Email, envelopeToken *Token, tks []*Token) error {
	charsetReader := func(label string, input io.Reader) (io.Reader, error) {
		label = strings.ReplaceAll(label, "windows-", "cp")
		encoding, _ := charset.Lookup(label)
		return encoding.NewDecoder().Reader(input), nil
	}
	dec := mime.WordDecoder{CharsetReader: charsetReader}

	if err := d.CheckType(envelopeToken, []TType{TContainer}, tks, "after ENVELOPE"); err != nil {
		return err
	}
	if err := d.CheckType(envelopeToken.Tokens[EDate], []TType{TQuoted, TNil}, tks, "for ENVELOPE[%d]", EDate); err != nil {
		return err
	}
	if err := d.CheckType(envelopeToken.Tokens[ESubject], []TType{TQuoted, TAtom, TNil}, tks, "for ENVELOPE[%d]", ESubject); err != nil {
		return err
	}

	e.Sent, _ = time.Parse("Mon, _2 Jan 2006 15:04:05 -0700", envelopeToken.Tokens[EDate].Str)
	e.Sent = e.Sent.UTC()

	var err error
	e.Subject, err = dec.DecodeHeader(envelopeToken.Tokens[ESubject].Str)
	if err != nil {
		return err
	}

	for _, a := range []struct {
		dest  *EmailAddresses
		pos   uint8
		debug string
	}{
		{&e.From, EFrom, "FROM"},
		{&e.ReplyTo, EReplyTo, "REPLYTO"},
		{&e.To, ETo, "TO"},
		{&e.CC, ECC, "CC"},
		{&e.BCC, EBCC, "BCC"},
	} {
		if err := d.parseEnvelopeAddresses(a.dest, envelopeToken.Tokens[a.pos], &dec, tks, a.debug); err != nil {
			return err
		}
	}

	e.MessageID = envelopeToken.Tokens[EMessageID].Str
	return nil
}

// parseEnvelopeAddresses parses a single address-list field from an ENVELOPE token.
func (d *Client) parseEnvelopeAddresses(dest *EmailAddresses, addrToken *Token, dec *mime.WordDecoder, tks []*Token, debug string) error {
	if addrToken.Type == TNil {
		return nil
	}
	if err := d.CheckType(addrToken, []TType{TNil, TContainer}, tks, "for ENVELOPE address %s", debug); err != nil {
		return err
	}
	*dest = make(map[string]string, len(addrToken.Tokens))
	for i, t := range addrToken.Tokens {
		if err := d.CheckType(t.Tokens[EEName], []TType{TQuoted, TAtom, TNil}, tks, "for %s[%d][%d]", debug, i, EEName); err != nil {
			return err
		}
		if err := d.CheckType(t.Tokens[EEMailbox], []TType{TQuoted, TAtom, TNil}, tks, "for %s[%d][%d]", debug, i, EEMailbox); err != nil {
			return err
		}
		if err := d.CheckType(t.Tokens[EEHost], []TType{TQuoted, TAtom, TNil}, tks, "for %s[%d][%d]", debug, i, EEHost); err != nil {
			return err
		}

		name, err := dec.DecodeHeader(t.Tokens[EEName].Str)
		if err != nil {
			return err
		}
		mailbox, err := dec.DecodeHeader(t.Tokens[EEMailbox].Str)
		if err != nil {
			return err
		}
		host, err := dec.DecodeHeader(t.Tokens[EEHost].Str)
		if err != nil {
			return err
		}
		(*dest)[strings.ToLower(mailbox+"@"+host)] = name
	}
	return nil
}

// parseOverviewField processes a single field (FLAGS, INTERNALDATE, RFC822.SIZE, ENVELOPE, UID)
// in an overview record at position i, returning the number of extra tokens to skip.
func (d *Client) parseOverviewField(e *Email, tks []*Token, i int, fieldName string) (skip int, err error) {
	switch fieldName {
	case "FLAGS":
		if err = d.CheckType(tks[i+1], []TType{TContainer}, tks, "after FLAGS"); err != nil {
			return 0, err
		}
		e.Flags = make([]string, len(tks[i+1].Tokens))
		for j, t := range tks[i+1].Tokens {
			if err = d.CheckType(t, []TType{TLiteral}, tks, "for FLAGS[%d]", j); err != nil {
				return 0, err
			}
			e.Flags[j] = t.Str
		}
		return 1, nil
	case "INTERNALDATE":
		if err = d.CheckType(tks[i+1], []TType{TQuoted}, tks, "after INTERNALDATE"); err != nil {
			return 0, err
		}
		e.Received, err = time.Parse(TimeFormat, tks[i+1].Str)
		if err != nil {
			return 0, err
		}
		e.Received = e.Received.UTC()
		return 1, nil
	case "RFC822.SIZE":
		if err = d.CheckType(tks[i+1], []TType{TNumber}, tks, "after RFC822.SIZE"); err != nil {
			return 0, err
		}
		e.Size = uint64(tks[i+1].Num)
		return 1, nil
	case "ENVELOPE":
		if err = d.parseEnvelope(e, tks[i+1], tks); err != nil {
			return 0, err
		}
		return 1, nil
	case "UID":
		if err = d.CheckType(tks[i+1], []TType{TNumber}, tks, "after UID"); err != nil {
			return 0, err
		}
		u, err := uidFromToken(tks[i+1].Num)
		if err != nil {
			return 0, err
		}
		e.UID = u
		return 1, nil
	}
	return 0, nil
}

// parseOverviewRecord processes a single FETCH record's tokens into an Email.
func (d *Client) parseOverviewRecord(tks []*Token) (*Email, error) {
	tks = unwrapTokens(tks)
	e := &Email{}
	skip := 0
	for i, t := range tks {
		if skip > 0 {
			skip--
			continue
		}
		if err := d.CheckType(t, []TType{TLiteral}, tks, "in root"); err != nil {
			return nil, err
		}
		s, err := d.parseOverviewField(e, tks, i, t.Str)
		if err != nil {
			return nil, err
		}
		skip = s
	}
	return e, nil
}

// GetOverviews retrieves email overview information (headers, flags, etc.).
func (d *Client) GetOverviews(ctx context.Context, uids ...UID) (emails map[UID]*Email, err error) {
	uidsStr := strings.Builder{}
	if len(uids) == 0 {
		uidsStr.WriteString("1:*")
	} else {
		for i, u := range uids {
			if u == 0 {
				continue
			}

			if i != 0 {
				uidsStr.WriteByte(',')
			}
			uidsStr.WriteString(u.String())
		}
	}

	var records [][]*Token
	err = retry.Retry(func() (err error) {
		r, err := d.Exec(ctx, "UID FETCH "+uidsStr.String()+" ALL", true, 0, nil)
		if err != nil {
			return err
		}

		if len(r) == 0 {
			return err
		}

		records, err = d.ParseFetchResponse(r)
		if err != nil {
			return err
		}
		return err
	}, d.effectiveRetryCount(), func(err error) error {
		errorLog(d.ConnNum, d.Folder, "fetch failed", "error", err)
		_ = d.Close()
		return nil
	}, func() error {
		return d.Reconnect(ctx)
	})
	if err != nil {
		return nil, err
	}

	emails = make(map[UID]*Email, len(uids))

	for _, tks := range records {
		e, err := d.parseOverviewRecord(tks)
		if err != nil {
			return nil, err
		}
		if e.UID > 0 {
			emails[e.UID] = e
		}
	}

	return emails, nil
}
