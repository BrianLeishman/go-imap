package imap

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"fmt"
	"io"
	"mime"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	retry "github.com/StirlingMarketingGroup/go-retry"
	"github.com/davecgh/go-spew/spew"
	humanize "github.com/dustin/go-humanize"
	"github.com/jhillyerd/enmime"
	"github.com/logrusorgru/aurora"
	"github.com/rs/xid"
	"golang.org/x/net/html/charset"
)

// AddSlashes adds slashes to double quotes
var AddSlashes = strings.NewReplacer(`"`, `\"`)

// RemoveSlashes removes slashes before double quotes
var RemoveSlashes = strings.NewReplacer(`\"`, `"`)

// Verbose outputs every command and its response with the IMAP server
var Verbose = false

// SkipResponses skips printing server responses in verbose mode
var SkipResponses = false

// RetryCount is the number of times retired functions get retried
var RetryCount = 10

var lastResp string

// Dialer is basically an IMAP connection
type Dialer struct {
	conn      *tls.Conn
	Folder    string
	Username  string
	Password  string
	Host      string
	Port      int
	strtokI   int
	strtok    string
	Connected bool
	ConnNum   int
}

// EmailAddresses are a map of email address to names
type EmailAddresses map[string]string

// Email is an email message
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

// Attachment is an Email attachment
type Attachment struct {
	Name     string
	MimeType string
	Content  []byte
}

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

func (a Attachment) String() string {
	return fmt.Sprintf("%s (%s %s)", a.Name, a.MimeType, humanize.Bytes(uint64(len(a.Content))))
}

var nextConnNum = 0
var nextConnNumMutex = sync.RWMutex{}

func log(connNum int, folder string, msg interface{}) {
	var name string
	if len(folder) != 0 {
		name = fmt.Sprintf("IMAP%d:%s", connNum, folder)
	} else {
		name = fmt.Sprintf("IMAP%d", connNum)
	}
	fmt.Println(aurora.Sprintf("%s %s: %s", time.Now().Format("2006-01-02 15:04:05.000000"), aurora.Colorize(name, aurora.CyanFg|aurora.BoldFm), msg))
}

// New makes a new imap
func New(username string, password string, host string, port int) (d *Dialer, err error) {
	nextConnNumMutex.RLock()
	connNum := nextConnNum
	nextConnNumMutex.RUnlock()

	nextConnNumMutex.Lock()
	nextConnNum++
	nextConnNumMutex.Unlock()

	err = retry.Retry(func() error {
		if Verbose {
			log(connNum, "", aurora.Green(aurora.Bold("establishing connection")))
		}
		var conn *tls.Conn
		conn, err = tls.Dial("tcp", host+":"+strconv.Itoa(port), nil)
		if err != nil {
			if Verbose {
				log(connNum, "", aurora.Red(aurora.Bold(fmt.Sprintf("failed to connect: %s", err))))
			}
			return err
		}
		d = &Dialer{
			conn:      conn,
			Username:  username,
			Password:  password,
			Host:      host,
			Port:      port,
			Connected: true,
			ConnNum:   connNum,
		}

		return d.Login(username, password)
	}, RetryCount, func(err error) error {
		if Verbose {
			log(connNum, "", aurora.Yellow(aurora.Bold("failed to establish connection, retrying shortly")))
			if d != nil && d.conn != nil {
				d.conn.Close()
			}
		}
		return nil
	}, func() error {
		if Verbose {
			log(connNum, "", aurora.Yellow(aurora.Bold("retrying failed connection now")))
		}
		return nil
	})
	if err != nil {
		if Verbose {
			log(connNum, "", aurora.Red(aurora.Bold("failed to establish connection")))
			if d.conn != nil {
				d.conn.Close()
			}
		}
		return nil, err
	}

	return
}

// Clone returns a new connection with the same connection information
// as the one this is being called on
func (d *Dialer) Clone() (d2 *Dialer, err error) {
	d2, err = New(d.Username, d.Password, d.Host, d.Port)
	// d2.Verbose = d1.Verbose
	if d.Folder != "" {
		err = d2.SelectFolder(d.Folder)
		if err != nil {
			return nil, fmt.Errorf("imap clone: %s", err)
		}
	}
	return
}

// Close closes the imap connection
func (d *Dialer) Close() (err error) {
	if d.Connected {
		if Verbose {
			log(d.ConnNum, d.Folder, aurora.Yellow(aurora.Bold("closing connection")))
		}
		err = d.conn.Close()
		if err != nil {
			return fmt.Errorf("imap close: %s", err)
		}
		d.Connected = false
	}
	return
}

// Reconnect closes the current connection (if any) and establishes a new one
func (d *Dialer) Reconnect() (err error) {
	d.Close()
	if Verbose {
		log(d.ConnNum, d.Folder, aurora.Yellow(aurora.Bold("reopening connection")))
	}
	d2, err := d.Clone()
	if err != nil {
		return fmt.Errorf("imap reconnect: %s", err)
	}
	*d = *d2
	return
}

const nl = "\r\n"

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

var atom = regexp.MustCompile(`{\d+}$`)

// Exec executes the command on the imap connection
func (d *Dialer) Exec(command string, buildResponse bool, retryCount int, processLine func(line []byte) error) (response string, err error) {
	var resp strings.Builder
	err = retry.Retry(func() (err error) {
		tag := []byte(fmt.Sprintf("%X", xid.New()))

		c := fmt.Sprintf("%s %s\r\n", tag, command)

		if Verbose {
			log(d.ConnNum, d.Folder, strings.Replace(fmt.Sprintf("%s %s", aurora.Bold("->"), strings.TrimSpace(c)), fmt.Sprintf(`"%s"`, d.Password), `"****"`, -1))
		}

		_, err = d.conn.Write([]byte(c))
		if err != nil {
			return
		}

		r := bufio.NewReader(d.conn)

		if buildResponse {
			resp = strings.Builder{}
		}
		var line []byte
		for err == nil {
			line, err = r.ReadBytes('\n')
			for {
				if a := atom.Find(dropNl(line)); a != nil {
					// fmt.Printf("%s\n", a)
					var n int
					n, err = strconv.Atoi(string(a[1 : len(a)-1]))
					if err != nil {
						return
					}

					buf := make([]byte, n)
					_, err = io.ReadFull(r, buf)
					if err != nil {
						return
					}
					line = append(line, buf...)

					buf, err = r.ReadBytes('\n')
					if err != nil {
						return
					}
					line = append(line, buf...)

					continue
				}
				break
			}

			if Verbose && !SkipResponses {
				log(d.ConnNum, d.Folder, fmt.Sprintf("<- %s", dropNl(line)))
			}

			// if strings.Contains(string(line), "--00000000000030095105741e7f1f") {
			// 	f, _ := ioutil.TempFile("", "")
			// 	ioutil.WriteFile(f.Name(), line, 0777)
			// 	fmt.Println(f.Name())
			// }

			if len(line) >= 19 && bytes.Equal(line[:16], tag) {
				if !bytes.Equal(line[17:19], []byte("OK")) {
					err = fmt.Errorf("imap command failed: %s", line[20:])
					return
				}
				break
			}

			if processLine != nil {
				if err = processLine(line); err != nil {
					return
				}
			}
			if buildResponse {
				resp.Write(line)
			}
		}
		return
	}, retryCount, func(err error) error {
		if Verbose {
			log(d.ConnNum, d.Folder, aurora.Red(err))
		}
		d.Close()
		return nil
	}, func() error {
		return d.Reconnect()
	})
	if err != nil {
		if Verbose {
			log(d.ConnNum, d.Folder, aurora.Red(aurora.Bold("All retries failed")))
		}
		return "", err
	}

	if buildResponse {
		if resp.Len() != 0 {
			lastResp = resp.String()
			return lastResp, nil
		}
		return "", nil
	}
	return
}

// Login attempts to login
func (d *Dialer) Login(username string, password string) (err error) {
	_, err = d.Exec(fmt.Sprintf(`LOGIN "%s" "%s"`, AddSlashes.Replace(username), AddSlashes.Replace(password)), false, RetryCount, nil)
	return
}

// GetFolders returns all folders
func (d *Dialer) GetFolders() (folders []string, err error) {
	folders = make([]string, 0)
	_, err = d.Exec(`LIST "" "*"`, false, RetryCount, func(line []byte) (err error) {
		line = dropNl(line)
		if b := bytes.IndexByte(line, '\n'); b != -1 {
			folders = append(folders, string(line[b+1:]))
		} else {
			if len(line) == 0 {
				return
			}
			i := len(line) - 1
			quoted := line[i] == '"'
			delim := byte(' ')
			if quoted {
				delim = '"'
				i--
			}
			end := i
			for i > 0 {
				if line[i] == delim {
					if !quoted || line[i-1] != '\\' {
						break
					}
				}
				i--
			}
			folders = append(folders, RemoveSlashes.Replace(string(line[i+1:end+1])))
		}
		return
	})
	if err != nil {
		return nil, err
	}

	return folders, nil
}

var regexExists = regexp.MustCompile(`\*\s+(\d+)\s+EXISTS`)

// GetTotalEmailCount returns the total number of emails in every folder
func (d *Dialer) GetTotalEmailCount() (count int, err error) {
	return d.GetTotalEmailCountStartingFromExcluding("", nil)
}

// GetTotalEmailCountExcluding returns the total number of emails in every folder
// excluding the specified folders
func (d *Dialer) GetTotalEmailCountExcluding(excludedFolders []string) (count int, err error) {
	return d.GetTotalEmailCountStartingFromExcluding("", excludedFolders)
}

// GetTotalEmailCountStartingFrom returns the total number of emails in every folder
// after the specified start folder
func (d *Dialer) GetTotalEmailCountStartingFrom(startFolder string) (count int, err error) {
	return d.GetTotalEmailCountStartingFromExcluding(startFolder, nil)
}

// GetTotalEmailCountStartingFromExcluding returns the total number of emails in every folder
// after the specified start folder, excluding the specified folders
func (d *Dialer) GetTotalEmailCountStartingFromExcluding(startFolder string, excludedFolders []string) (count int, err error) {
	started := true
	if len(startFolder) != 0 {
		started = false
	}

	folder := d.Folder

	folders, err := d.GetFolders()
	if err != nil {
		return
	}

	for _, f := range folders {
		if !started {
			if f == startFolder {
				started = true
			} else {
				continue
			}
		}

		skip := false
		for _, ef := range excludedFolders {
			if f == ef {
				skip = true
				break
			}
		}
		if skip {
			continue
		}

		err = d.SelectFolder(f)
		if err != nil {
			return
		}

		var n int
		n, err = strconv.Atoi(regexExists.FindStringSubmatch(lastResp)[1])
		if err != nil {
			return
		}

		count += n
	}

	if len(folder) != 0 {
		err = d.SelectFolder(folder)
		if err != nil {
			return
		}
	}

	return
}

// SelectFolder selects a folder
func (d *Dialer) SelectFolder(folder string) (err error) {
	_, err = d.Exec(`EXAMINE "`+AddSlashes.Replace(folder)+`"`, true, RetryCount, nil)
	if err != nil {
		return
	}
	d.Folder = folder
	return nil
}

// GetUIDs returns the UIDs in the current folder that match the search
func (d *Dialer) GetUIDs(search string) (uids []int, err error) {
	uids = make([]int, 0)
	t := []byte{' ', '\r', '\n'}
	r, err := d.Exec(`UID SEARCH `+search, true, RetryCount, nil)
	if err != nil {
		return nil, err
	}
	if d.StrtokInit(r, t) == "*" && d.Strtok(t) == "SEARCH" {
		for {
			uid := string(d.Strtok(t))
			if len(uid) == 0 {
				break
			}
			u, err := strconv.Atoi(string(uid))
			if err != nil {
				return nil, err
			}
			uids = append(uids, u)
		}
	}

	return uids, nil
}

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
	// EESR is unused and should be ignored
	EESR
	EEMailbox
	EEHost
)

// GetEmails returns email with their bodies for the given UIDs in the current folder.
// If no UIDs are given, they everything in the current folder is selected
func (d *Dialer) GetEmails(uids ...int) (emails map[int]*Email, err error) {
	emails, err = d.GetOverviews(uids...)
	if err != nil {
		return nil, err
	}

	if len(emails) == 0 {
		return
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
			return
		}

		records, err = d.ParseFetchResponse(r)
		if err != nil {
			return
		}

		for _, tks := range records {
			e := &Email{}
			skip := 0
			success := true
			for i, t := range tks {
				if skip > 0 {
					skip--
					continue
				}
				if err = d.CheckType(t, []TType{TLiteral}, tks, "in root"); err != nil {
					return
				}
				switch t.Str {
				case "BODY[]":
					if err = d.CheckType(tks[i+1], []TType{TAtom}, tks, "after BODY[]"); err != nil {
						return
					}
					msg := tks[i+1].Str
					r := strings.NewReader(msg)

					env, err := enmime.ReadEnvelope(r)
					if err != nil {
						if Verbose {
							log(d.ConnNum, d.Folder, aurora.Yellow(aurora.Sprintf("email body could not be parsed, skipping: %s", err)))
							spew.Dump(env)
							spew.Dump(msg)
							// os.Exit(0)
						}
						success = false

						// continue RecL
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
						return
					}
					e.UID = tks[i+1].Num
					skip++
				}
			}

			if success {
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
		return
	}, RetryCount, func(err error) error {
		log(d.ConnNum, d.Folder, aurora.Red(aurora.Bold(err)))
		d.Close()
		return nil
	}, func() error {
		return d.Reconnect()
	})

	return
}

// GetOverviews returns emails without bodies for the given UIDs in the current folder.
// If no UIDs are given, they everything in the current folder is selected
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
			return
		}

		if len(r) == 0 {
			return
		}

		records, err = d.ParseFetchResponse(r)
		if err != nil {
			return
		}
		return
	}, RetryCount, func(err error) error {
		log(d.ConnNum, d.Folder, aurora.Red(aurora.Bold(err)))
		d.Close()
		return nil
	}, func() error {
		return d.Reconnect()
	})
	if err != nil {
		return nil, err
	}

	emails = make(map[int]*Email, len(uids))
	CharsetReader := func(label string, input io.Reader) (io.Reader, error) {
		label = strings.Replace(label, "windows-", "cp", -1)
		encoding, _ := charset.Lookup(label)
		return encoding.NewDecoder().Reader(input), nil
	}
	dec := mime.WordDecoder{CharsetReader: CharsetReader}

	// RecordsL:
	for _, tks := range records {
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
					if tks[i+1].Tokens[EFrom].Type != TNil {
						if err = d.CheckType(tks[i+1].Tokens[a.pos], []TType{TNil, TContainer}, tks, "for ENVELOPE[%d]", a.pos); err != nil {
							return nil, err
						}
						*a.dest = make(map[string]string, len(tks[i+1].Tokens[EFrom].Tokens))
						for i, t := range tks[i+1].Tokens[a.pos].Tokens {
							if err = d.CheckType(t.Tokens[EEName], []TType{TQuoted, TNil}, tks, "for %s[%d][%d]", a.debug, i, EEName); err != nil {
								return nil, err
							}
							if err = d.CheckType(t.Tokens[EEMailbox], []TType{TQuoted, TNil}, tks, "for %s[%d][%d]", a.debug, i, EEMailbox); err != nil {
								return nil, err
							}
							if err = d.CheckType(t.Tokens[EEHost], []TType{TQuoted, TNil}, tks, "for %s[%d][%d]", a.debug, i, EEHost); err != nil {
								return nil, err
							}

							name, err := dec.DecodeHeader(t.Tokens[EEName].Str)
							if err != nil {
								return nil, err
							}

							// if t.Tokens[EEMailbox].Type == TNil {
							// 	if Verbose {
							// 		log(d.ConnNum, d.Folder, Brown("email address has no mailbox name (probably not a real email), skipping"))
							// 	}
							// 	continue RecordsL
							// }
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

		emails[e.UID] = e
	}

	return
}

// Token is a fetch response token (e.g. a number, or a quoted section, or a container, etc.)
type Token struct {
	Type   TType
	Str    string
	Num    int
	Tokens []*Token
}

// TType is the enum type for token values
type TType uint8

const (
	// TUnset is an unset token; used by the parser
	TUnset TType = iota
	// TAtom is a string that's prefixed with `{n}`
	// where n is the number of bytes in the string
	TAtom
	// TNumber is a numeric literal
	TNumber
	// TLiteral is a literal (think string, ish, used mainly for field names, I hope)
	TLiteral
	// TQuoted is a quoted piece of text
	TQuoted
	// TNil is a nil value, nothing
	TNil
	// TContainer is a container of tokens
	TContainer
)

// TimeFormat is the Go time version of the IMAP times
const TimeFormat = "_2-Jan-2006 15:04:05 -0700"

type tokenContainer *[]*Token

// ParseFetchResponse parses a response from a FETCH command into tokens
func (d *Dialer) ParseFetchResponse(r string) (records [][]*Token, err error) {
	records = make([][]*Token, 0)
	for {
		t := []byte{' ', '\r', '\n'}
		ok := false
		if string(d.StrtokInit(r, t)) == "*" {
			if _, err := strconv.Atoi(string(d.Strtok(t))); err == nil && string(d.Strtok(t)) == "FETCH" {
				ok = true
			}
		}

		if !ok {
			return nil, fmt.Errorf("unable to parse Fetch line %#v", string(r[:d.GetStrtokI()]))
		}

		tokens := make([]*Token, 0)
		r = r[d.GetStrtokI()+1:]

		currentToken := TUnset
		tokenStart := 0
		tokenEnd := 0
		// escaped := false
		depth := 0
		container := make([]tokenContainer, 4)
		container[0] = &tokens

		pushToken := func() *Token {
			var t *Token
			switch currentToken {
			case TQuoted:
				t = &Token{
					Type: currentToken,
					Str:  RemoveSlashes.Replace(string(r[tokenStart : tokenEnd+1])),
				}
			case TLiteral:
				s := string(r[tokenStart : tokenEnd+1])
				num, err := strconv.Atoi(s)
				if err == nil {
					t = &Token{
						Type: TNumber,
						Num:  num,
					}
				} else {
					if s == "NIL" {
						t = &Token{
							Type: TNil,
						}
					} else {
						t = &Token{
							Type: TLiteral,
							Str:  s,
						}
					}
				}
			case TAtom:
				t = &Token{
					Type: currentToken,
					Str:  string(r[tokenStart : tokenEnd+1]),
				}
			case TContainer:
				t = &Token{
					Type:   currentToken,
					Tokens: make([]*Token, 0, 1),
				}
			}

			if t != nil {
				*container[depth] = append(*container[depth], t)
			}
			currentToken = TUnset

			return t
		}

		l := len(r)
		i := 0
		for i < l {
			b := r[i]

			switch currentToken {
			case TQuoted:
				switch b {
				case '"':
					tokenEnd = i - 1
					pushToken()
					goto Cont
				case '\\':
					i++
					goto Cont
				}
			case TLiteral:
				switch {
				case IsLiteral(rune(b)):
				default:
					tokenEnd = i - 1
					pushToken()
				}
			case TAtom:
				switch {
				case unicode.IsDigit(rune(b)):
				default:
					tokenEnd = i
					size, err := strconv.Atoi(string(r[tokenStart:tokenEnd]))
					if err != nil {
						return nil, err
					}
					i += len("}") + len(nl)
					tokenStart = i
					tokenEnd = tokenStart + size - 1
					i = tokenEnd
					pushToken()
				}
			}

			switch currentToken {
			case TUnset:
				switch {
				case b == '"':
					currentToken = TQuoted
					tokenStart = i + 1
				case IsLiteral(rune(b)):
					currentToken = TLiteral
					tokenStart = i
				case b == '{':
					currentToken = TAtom
					tokenStart = i + 1
				case b == '(':
					currentToken = TContainer
					t := pushToken()
					depth++
					container[depth] = &t.Tokens
				case b == ')':
					depth--
				}
			}

		Cont:
			if depth < 0 {
				break
			}
			i++
			if i >= l {
				tokenEnd = l
				pushToken()
			}
		}
		records = append(records, tokens)
		r = r[i+1+len(nl):]

		if len(r) == 0 {
			break
		}
	}

	return
}

// IsLiteral returns if the given byte is an acceptable literal character
func IsLiteral(b rune) bool {
	switch {
	case unicode.IsDigit(b),
		unicode.IsLetter(b),
		b == '\\',
		b == '.',
		b == '[',
		b == ']':
		return true
	}
	return false
}

// GetTokenName returns the name of the given token type token
func GetTokenName(tokenType TType) string {
	switch tokenType {
	case TUnset:
		return "TUnset"
	case TAtom:
		return "TAtom"
	case TNumber:
		return "TNumber"
	case TLiteral:
		return "TLiteral"
	case TQuoted:
		return "TQuoted"
	case TNil:
		return "TNil"
	case TContainer:
		return "TContainer"
	}
	return ""
}

func (t Token) String() string {
	tokenType := GetTokenName(t.Type)
	switch t.Type {
	case TUnset, TNil:
		return tokenType
	case TAtom, TQuoted:
		return fmt.Sprintf("(%s, len %d, chars %d %#v)", tokenType, len(t.Str), len([]rune(t.Str)), t.Str)
	case TNumber:
		return fmt.Sprintf("(%s %d)", tokenType, t.Num)
	case TLiteral:
		return fmt.Sprintf("(%s %s)", tokenType, t.Str)
	case TContainer:
		return fmt.Sprintf("(%s children: %s)", tokenType, t.Tokens)
	}
	return ""
}

// CheckType validates a type against a list of acceptable types,
// if the type of the token isn't in the list, an error is returned
func (d *Dialer) CheckType(token *Token, acceptableTypes []TType, tks []*Token, loc string, v ...interface{}) (err error) {
	ok := false
	for _, a := range acceptableTypes {
		if token.Type == a {
			ok = true
			break
		}
	}
	if !ok {
		types := ""
		for i, a := range acceptableTypes {
			if i != 0 {
				types += "|"
			}
			types += GetTokenName(a)
		}
		err = fmt.Errorf("IMAP%d:%s: expected %s token %s, got %+v in %v", d.ConnNum, d.Folder, types, fmt.Sprintf(loc, v...), token, tks)
	}

	return err
}
