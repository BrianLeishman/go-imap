package imap

import (
	"fmt"
	"io"
	"mime"
	"reflect"
	"strconv"
	"strings"
	"time"

	retry "github.com/StirlingMarketingGroup/go-retry"
	"github.com/davecgh/go-spew/spew"
	humanize "github.com/dustin/go-humanize"
	"github.com/jhillyerd/enmime"
	"golang.org/x/net/html/charset"
)

// EmailAddresses represents a map of email addresses to display names
type EmailAddresses map[string]string

// Email represents an IMAP email message
type Email struct {
	Flags       []string
	Received    time.Time
	Sent        time.Time
	Size        uint64
	Subject     string
	UID         int
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
				emails.WriteString(fmt.Sprintf(`"%s" <%s>`, AddSlashes.Replace(n), e))
			} else {
				emails.WriteString(fmt.Sprintf(`%s <%s>`, n, e))
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

	email.WriteString(fmt.Sprintf("Subject: %s\n", e.Subject))

	if len(e.To) != 0 {
		email.WriteString(fmt.Sprintf("To: %s\n", e.To))
	}
	if len(e.From) != 0 {
		email.WriteString(fmt.Sprintf("From: %s\n", e.From))
	}
	if len(e.CC) != 0 {
		email.WriteString(fmt.Sprintf("CC: %s\n", e.CC))
	}
	if len(e.BCC) != 0 {
		email.WriteString(fmt.Sprintf("BCC: %s\n", e.BCC))
	}
	if len(e.ReplyTo) != 0 {
		email.WriteString(fmt.Sprintf("ReplyTo: %s\n", e.ReplyTo))
	}
	if len(e.Text) != 0 {
		if len(e.Text) > 20 {
			email.WriteString(fmt.Sprintf("Text: %s...", e.Text[:20]))
		} else {
			email.WriteString(fmt.Sprintf("Text: %s", e.Text))
		}
		email.WriteString(fmt.Sprintf("(%s)\n", humanize.Bytes(uint64(len(e.Text)))))
	}
	if len(e.HTML) != 0 {
		if len(e.HTML) > 20 {
			email.WriteString(fmt.Sprintf("HTML: %s...", e.HTML[:20]))
		} else {
			email.WriteString(fmt.Sprintf("HTML: %s", e.HTML))
		}
		email.WriteString(fmt.Sprintf(" (%s)\n", humanize.Bytes(uint64(len(e.HTML)))))
	}

	if len(e.Attachments) != 0 {
		email.WriteString(fmt.Sprintf("%d Attachment(s): %s\n", len(e.Attachments), e.Attachments))
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
func (d *Dialer) GetUIDs(search string) (uids []int, err error) {
	r, err := d.Exec(`UID SEARCH `+search, true, RetryCount, nil)
	if err != nil {
		return nil, err
	}
	return parseUIDSearchResponse(r)
}

// GetLastNUIDs returns the N messages with the highest UIDs in the selected folder.
// This is useful for fetching the most recent messages.
//
// Example:
//
//	// Get the 10 most recent messages
//	uids, err := conn.GetLastNUIDs(10)
func (d *Dialer) GetLastNUIDs(n int) ([]int, error) {
	if n <= 0 {
		return nil, nil
	}
	allUIDs, err := d.GetUIDs("ALL")
	if err != nil {
		return nil, err
	}
	if len(allUIDs) <= n {
		return allUIDs, nil
	}
	return allUIDs[len(allUIDs)-n:], nil
}

// MoveEmail moves an email to a different folder
func (d *Dialer) MoveEmail(uid int, folder string) (err error) {
	// if we are currently read-only, switch to SELECT for the move-operation
	readOnlyState := d.ReadOnly
	if readOnlyState {
		_ = d.SelectFolder(d.Folder)
	}
	_, err = d.Exec(`UID MOVE `+strconv.Itoa(uid)+` "`+AddSlashes.Replace(folder)+`"`, true, RetryCount, nil)
	if readOnlyState {
		_ = d.ExamineFolder(d.Folder)
	}
	if err != nil {
		return err
	}
	d.Folder = folder
	return nil
}

// MarkSeen marks an email as seen/read
func (d *Dialer) MarkSeen(uid int) (err error) {
	flags := Flags{
		Seen: FlagAdd,
	}

	readOnlyState := d.ReadOnly
	if readOnlyState {
		_ = d.SelectFolder(d.Folder)
	}
	err = d.SetFlags(uid, flags)
	if readOnlyState {
		_ = d.ExamineFolder(d.Folder)
	}

	return err
}

// DeleteEmail marks an email for deletion
func (d *Dialer) DeleteEmail(uid int) (err error) {
	flags := Flags{
		Deleted: FlagAdd,
	}

	readOnlyState := d.ReadOnly
	if readOnlyState {
		if err = d.SelectFolder(d.Folder); err != nil {
			return err
		}
	}
	err = d.SetFlags(uid, flags)
	if readOnlyState {
		if e := d.ExamineFolder(d.Folder); e != nil && err == nil {
			err = e
		}
	}

	return err
}

// Expunge permanently removes emails marked for deletion
func (d *Dialer) Expunge() (err error) {
	readOnlyState := d.ReadOnly
	if readOnlyState {
		if err = d.SelectFolder(d.Folder); err != nil {
			return err
		}
	}
	_, err = d.Exec("EXPUNGE", false, RetryCount, nil)
	if readOnlyState {
		if e := d.ExamineFolder(d.Folder); e != nil && err == nil {
			err = e
		}
	}
	return err
}

// SetFlags sets message flags (seen, deleted, etc.)
func (d *Dialer) SetFlags(uid int, flags Flags) (err error) {
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
		_ = d.SelectFolder(d.Folder)
	}
	_, err = d.Exec(query, true, RetryCount, nil)
	if readOnlyState {
		_ = d.ExamineFolder(d.Folder)
	}

	return err
}

// GetEmails retrieves full email messages including body content
func (d *Dialer) GetEmails(uids ...int) (emails map[int]*Email, err error) {
	emails, err = d.GetOverviews(uids...)
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
			uidsStr.WriteString(strconv.Itoa(u))
			i++
		}
	}

	var records [][]*Token
	err = retry.Retry(func() (err error) {
		r, err := d.Exec("UID FETCH "+uidsStr.String()+" BODY.PEEK[]", true, 0, nil)
		if err != nil {
			return err
		}

		records, err = d.ParseFetchResponse(r)
		if err != nil {
			return err
		}

		for _, tks := range records {
			// Some servers may wrap the FETCH content with extra parentheses.
			// Flatten single-child containers defensively until we reach fields.
			for len(tks) == 1 && tks[0].Type == TContainer {
				tks = tks[0].Tokens
			}
			e := &Email{}
			skip := 0
			success := true
			for i, t := range tks {
				if skip > 0 {
					skip--
					continue
				}
				if err = d.CheckType(t, []TType{TLiteral}, tks, "in root"); err != nil {
					return err
				}
				switch t.Str {
				case "BODY[]":
					if err = d.CheckType(tks[i+1], []TType{TAtom}, tks, "after BODY[]"); err != nil {
						return err
					}
					msg := tks[i+1].Str
					r := strings.NewReader(msg)

					env, err := enmime.ReadEnvelope(r)
					if err != nil {
						if Verbose {
							warnLog(d.ConnNum, d.Folder, "email body could not be parsed", "error", err)
							spew.Dump(env)
							spew.Dump(msg)
						}
						success = false
					} else {
						e.Subject = env.GetHeader("Subject")
						e.Text = env.Text
						e.HTML = env.HTML

						if len(env.Attachments) != 0 {
							for _, a := range env.Attachments {
								e.Attachments = append(e.Attachments, Attachment{
									Name:     a.FileName,
									MimeType: a.ContentType,
									Content:  a.Content,
								})
							}
						}

						if len(env.Inlines) != 0 {
							for _, a := range env.Inlines {
								e.Attachments = append(e.Attachments, Attachment{
									Name:     a.FileName,
									MimeType: a.ContentType,
									Content:  a.Content,
								})
							}
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
					}
					skip++
				case "UID":
					if err = d.CheckType(tks[i+1], []TType{TNumber}, tks, "after UID"); err != nil {
						return err
					}
					e.UID = tks[i+1].Num
					skip++
				}
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
	}, RetryCount, func(err error) error {
		errorLog(d.ConnNum, d.Folder, "fetch failed", "error", err)
		_ = d.Close()
		return nil
	}, func() error {
		return d.Reconnect()
	})

	return emails, err
}

// GetOverviews retrieves email overview information (headers, flags, etc.)
func (d *Dialer) GetOverviews(uids ...int) (emails map[int]*Email, err error) {
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
			uidsStr.WriteString(strconv.Itoa(u))
		}
	}

	var records [][]*Token
	err = retry.Retry(func() (err error) {
		r, err := d.Exec("UID FETCH "+uidsStr.String()+" ALL", true, 0, nil)
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
	}, RetryCount, func(err error) error {
		errorLog(d.ConnNum, d.Folder, "fetch failed", "error", err)
		_ = d.Close()
		return nil
	}, func() error {
		return d.Reconnect()
	})
	if err != nil {
		return nil, err
	}

	emails = make(map[int]*Email, len(uids))

	for _, tks := range records {
		for len(tks) == 1 && tks[0].Type == TContainer {
			tks = tks[0].Tokens
		}
		e := &Email{}
		skip := 0
		for i, t := range tks {
			if skip > 0 {
				skip--
				continue
			}
			if err = d.CheckType(t, []TType{TLiteral}, tks, "in root"); err != nil {
				return nil, err
			}
			switch t.Str {
			case "FLAGS":
				if err = d.CheckType(tks[i+1], []TType{TContainer}, tks, "after FLAGS"); err != nil {
					return nil, err
				}
				e.Flags = make([]string, len(tks[i+1].Tokens))
				for i, t := range tks[i+1].Tokens {
					if err = d.CheckType(t, []TType{TLiteral}, tks, "for FLAGS[%d]", i); err != nil {
						return nil, err
					}
					e.Flags[i] = t.Str
				}
				skip++
			case "INTERNALDATE":
				if err = d.CheckType(tks[i+1], []TType{TQuoted}, tks, "after INTERNALDATE"); err != nil {
					return nil, err
				}
				e.Received, err = time.Parse(TimeFormat, tks[i+1].Str)
				if err != nil {
					return nil, err
				}
				e.Received = e.Received.UTC()
				skip++
			case "RFC822.SIZE":
				if err = d.CheckType(tks[i+1], []TType{TNumber}, tks, "after RFC822.SIZE"); err != nil {
					return nil, err
				}
				e.Size = uint64(tks[i+1].Num)
				skip++
			case "ENVELOPE":
				CharsetReader := func(label string, input io.Reader) (io.Reader, error) {
					label = strings.ReplaceAll(label, "windows-", "cp")
					encoding, _ := charset.Lookup(label)
					return encoding.NewDecoder().Reader(input), nil
				}
				dec := mime.WordDecoder{CharsetReader: CharsetReader}

				if err = d.CheckType(tks[i+1], []TType{TContainer}, tks, "after ENVELOPE"); err != nil {
					return nil, err
				}
				if err = d.CheckType(tks[i+1].Tokens[EDate], []TType{TQuoted, TNil}, tks, "for ENVELOPE[%d]", EDate); err != nil {
					return nil, err
				}
				if err = d.CheckType(tks[i+1].Tokens[ESubject], []TType{TQuoted, TAtom, TNil}, tks, "for ENVELOPE[%d]", ESubject); err != nil {
					return nil, err
				}

				e.Sent, _ = time.Parse("Mon, _2 Jan 2006 15:04:05 -0700", tks[i+1].Tokens[EDate].Str)
				e.Sent = e.Sent.UTC()

				e.Subject, err = dec.DecodeHeader(tks[i+1].Tokens[ESubject].Str)
				if err != nil {
					return nil, err
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
					if tks[i+1].Tokens[a.pos].Type != TNil {
						if err = d.CheckType(tks[i+1].Tokens[a.pos], []TType{TNil, TContainer}, tks, "for ENVELOPE[%d]", a.pos); err != nil {
							return nil, err
						}
						*a.dest = make(map[string]string, len(tks[i+1].Tokens[a.pos].Tokens))
						for i, t := range tks[i+1].Tokens[a.pos].Tokens {
							if err = d.CheckType(t.Tokens[EEName], []TType{TQuoted, TAtom, TNil}, tks, "for %s[%d][%d]", a.debug, i, EEName); err != nil {
								return nil, err
							}
							if err = d.CheckType(t.Tokens[EEMailbox], []TType{TQuoted, TAtom, TNil}, tks, "for %s[%d][%d]", a.debug, i, EEMailbox); err != nil {
								return nil, err
							}
							if err = d.CheckType(t.Tokens[EEHost], []TType{TQuoted, TAtom, TNil}, tks, "for %s[%d][%d]", a.debug, i, EEHost); err != nil {
								return nil, err
							}

							name, err := dec.DecodeHeader(t.Tokens[EEName].Str)
							if err != nil {
								return nil, err
							}

							mailbox, err := dec.DecodeHeader(t.Tokens[EEMailbox].Str)
							if err != nil {
								return nil, err
							}

							host, err := dec.DecodeHeader(t.Tokens[EEHost].Str)
							if err != nil {
								return nil, err
							}

							(*a.dest)[strings.ToLower(mailbox+"@"+host)] = name
						}
					}
				}

				e.MessageID = tks[i+1].Tokens[EMessageID].Str

				skip++
			case "UID":
				if err = d.CheckType(tks[i+1], []TType{TNumber}, tks, "after UID"); err != nil {
					return nil, err
				}
				e.UID = tks[i+1].Num
				skip++
			}
		}

		if e.UID > 0 {
			emails[e.UID] = e
		}
	}

	return emails, nil
}
