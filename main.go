package imap

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"fmt"
	"io"
	"mime"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/StirlingMarketingGroup/go-retry"
	"github.com/jhillyerd/enmime"
	. "github.com/logrusorgru/aurora"
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

// Dialer is basically an IMAP connection
type Dialer struct {
	conn   *tls.Conn
	Folder string
	// Verbose     bool
	Username    string
	Password    string
	Host        string
	Port        int
	strtokI     int
	strtokBytes []byte
	Connected   bool
	ConnNum     int
}

var nextConnNum = 0
var nextConnNumMutex = sync.RWMutex{}

func log(connNum int, msg interface{}) {
	fmt.Println(Sprintf("%s %s: %s", time.Now().Format("2006-01-02 15:04:05.000000"), Colorize(fmt.Sprintf("IMAPConn%d", connNum), CyanFg|BoldFm), msg))
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
			log(connNum, Green(Bold("establishing connection")))
		}
		var conn *tls.Conn
		conn, err = tls.Dial("tcp", host+":"+strconv.Itoa(port), nil)
		if err != nil {
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
	}, 3, func() error {
		if Verbose {
			log(connNum, Brown(Bold("failed to establish connection, retrying shortly")))
		}
		return nil
	}, func() error {
		if Verbose {
			log(connNum, Brown(Bold("retrying failed connection now")))
		}
		return nil
	})
	if err != nil {
		if Verbose {
			log(connNum, Red(Bold("failed to establish connection")))
		}
		return nil, err
	}

	return
}

// Clone returns a new connection with the same conneciton information
// as the one this is being called on
func (d *Dialer) Clone() (d2 *Dialer, err error) {
	d2, err = New(d.Username, d.Password, d.Host, d.Port)
	// d2.Verbose = d1.Verbose
	if d.Folder != "" {
		err = d2.SelectFolder(d.Folder)
		if err != nil {
			return nil, err
		}
	}
	return
}

// Close closes the imap connection
func (d *Dialer) Close() (err error) {
	if d.Connected {
		if Verbose {
			log(d.ConnNum, Brown(Bold("closing connection")))
		}
		err = d.conn.Close()
		if err != nil {
			return err
		}
		d.Connected = false
	}
	return
}

// Reconnect closes the current connection (if any) and establishes a new one
func (d *Dialer) Reconnect() (err error) {
	d.Close()
	if Verbose {
		log(d.ConnNum, Brown(Bold("reopening connection")))
	}
	d, err = d.Clone()
	if err != nil {
		return err
	}
	return
}

const nl = "\r\n"

// Exec executes the command on the imap connection
func (d *Dialer) Exec(command string, buildResponse bool, newlinesBetweenLines bool, processLine func(line []byte) error) (response []byte, err error) {
	var buf *bytes.Buffer
	err = retry.Retry(func() (err error) {
		tag := fmt.Sprintf("%X", bid2())

		c := fmt.Sprintf("%s %s\r\n", tag, command)

		if Verbose {
			log(d.ConnNum, strings.Replace(fmt.Sprintf("%s %s", Bold("->"), strings.TrimSpace(c)), fmt.Sprintf(`"%s"`, d.Password), `"****"`, -1))
		}

		_, err = d.conn.Write([]byte(c))
		if err != nil {
			return
		}

		r := bufio.NewReader(d.conn)

		if buildResponse {
			buf = bytes.NewBuffer(nil)
		}
		for {
			var line []byte
			line, _, err = r.ReadLine()
			if err != nil {
				return
			}

			if Verbose && !SkipResponses {
				log(d.ConnNum, fmt.Sprintf("<- %s", string(line)))
			}

			if len(line) >= 19 && string(line[:16]) == tag {
				if string(line[17:19]) != "OK" {
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
				buf.Write(line)
				if newlinesBetweenLines {
					buf.WriteString(nl)
				}
			}
		}
		return
	}, 3, func() error {
		return d.Close()
	}, func() error {
		return d.Reconnect()
	})
	if err != nil {
		return nil, err
	}

	if buildResponse {
		if buf != nil {
			return buf.Bytes(), nil
		}
		return []byte{}, nil
	}
	return
}

// Login attempts to login
func (d *Dialer) Login(username string, password string) (err error) {
	_, err = d.Exec(fmt.Sprintf(`LOGIN "%s" "%s"`, AddSlashes.Replace(username), AddSlashes.Replace(password)), false, false, nil)
	return
}

// GetFolders returns all folders
func (d *Dialer) GetFolders() (folders []string, err error) {
	folders = make([]string, 0)
	nextLineIsFolder := false
	_, err = d.Exec(`LIST "" "*"`, false, false, func(line []byte) error {
		if nextLineIsFolder {
			folders = append(folders, string(line))
			nextLineIsFolder = false
		} else {
			i := len(line) - 1
			quoted := line[i] == '"'
			if line[i] == '}' {
				nextLineIsFolder = true
				return nil
			}
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
		return nil
	})
	if err != nil {
		return nil, err
	}

	return folders, nil
}

// SelectFolder selects a folder
func (d *Dialer) SelectFolder(folder string) (err error) {
	_, err = d.Exec(`EXAMINE "`+AddSlashes.Replace(folder)+`"`, false, false, nil)
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
	r, err := d.Exec(`UID SEARCH `+search, true, false, nil)
	if err != nil {
		return nil, err
	}
	if string(d.StrtokInit(r, t)) == "*" && string(d.Strtok(t)) == "SEARCH" {
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

// Email is an email message
type Email struct {
	Flags     []string
	Received  time.Time
	Sent      time.Time
	Size      uint64
	Subject   string
	UID       int
	MessageID string
	From      map[string]string
	To        map[string]string
	ReplyTo   map[string]string
	CC        map[string]string
	BCC       map[string]string
	Text      string
	HTML      string
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
			if i != 0 {
				uidsStr.WriteByte(',')
			}
			uidsStr.WriteString(strconv.Itoa(u))
			i++
		}
	}

	r, err := d.Exec("UID FETCH "+uidsStr.String()+" BODY.PEEK[]", true, true, nil)
	if err != nil {
		return nil, err
	}

	records, err := d.ParseFetchResponse(r)
	if err != nil {
		return nil, err
	}

	// RecL:
	for _, tks := range records {
		e := &Email{}
		skip := 0
		success := true
		for i, t := range tks {
			if skip > 0 {
				skip--
				continue
			}
			if err = CheckType(t, []TType{TLiteral}, "in root"); err != nil {
				return nil, err
			}
			switch t.Str {
			case "BODY[]":
				if err = CheckType(tks[i+1], []TType{TAtom}, "after BODY[]"); err != nil {
					return nil, err
				}
				msg := tks[i+1].Str
				r := strings.NewReader(msg)

				env, _ := enmime.ReadEnvelope(r)
				if env == nil {
					if Verbose {
						log(d.ConnNum, Brown("email body could not be parsed, skipping"))
					}
					success = false
					// continue RecL
				}

				e.Subject = env.GetHeader("Subject")
				e.Text = env.Text
				e.HTML = env.HTML

				for _, a := range []struct {
					dest   *map[string]string
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

				skip++
			case "UID":
				if err = CheckType(tks[i+1], []TType{TNumber}, "after UID"); err != nil {
					return nil, err
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
		} else {
			delete(emails, e.UID)
		}
	}

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
			if i != 0 {
				uidsStr.WriteByte(',')
			}
			uidsStr.WriteString(strconv.Itoa(u))
		}
	}

	var records [][]*Token
	err = retry.Retry(func() (err error) {
		r, err := d.Exec("UID FETCH "+uidsStr.String()+" ALL", true, true, nil)
		if err != nil {
			return
		}

		records, err = d.ParseFetchResponse(r)
		if err != nil {
			return
		}
		return
	}, 3, func() error {
		return d.Close()
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

RecordsL:
	for _, tokens := range records {
		e := &Email{}
		skip := 0
		for i, t := range tokens {
			if skip > 0 {
				skip--
				continue
			}
			if t.Depth == 0 {
				if err = CheckType(t, []TType{TLiteral}, "in root"); err != nil {
					return nil, err
				}
				switch t.Str {
				case "FLAGS":
					if err = CheckType(tokens[i+1], []TType{TContainer}, "after FLAGS"); err != nil {
						return nil, err
					}
					e.Flags = make([]string, len(tokens[i+1].Tokens))
					for i, t := range tokens[i+1].Tokens {
						if err = CheckType(t, []TType{TLiteral}, "for FLAGS[%d]", i); err != nil {
							return nil, err
						}
						e.Flags[i] = t.Str
					}
					skip++
				case "INTERNALDATE":
					if err = CheckType(tokens[i+1], []TType{TQuoted}, "after INTERNALDATE"); err != nil {
						return nil, err
					}
					e.Received, err = time.Parse(TimeFormat, tokens[i+1].Str)
					if err != nil {
						return nil, err
					}
					e.Received = e.Received.UTC()
					skip++
				case "RFC822.SIZE":
					if err = CheckType(tokens[i+1], []TType{TNumber}, "after RFC822.SIZE"); err != nil {
						return nil, err
					}
					e.Size = uint64(tokens[i+1].Num)
					skip++
				case "ENVELOPE":
					if err = CheckType(tokens[i+1], []TType{TContainer}, "after ENVELOPE"); err != nil {
						return nil, err
					}
					if err = CheckType(tokens[i+1].Tokens[EDate], []TType{TQuoted, TNil}, "for ENVELOPE[%d]", EDate); err != nil {
						return nil, err
					}
					if err = CheckType(tokens[i+1].Tokens[ESubject], []TType{TQuoted}, "for ENVELOPE[%d]", ESubject); err != nil {
						return nil, err
					}

					e.Sent, _ = time.Parse("Mon, _2 Jan 2006 15:04:05 -0700", tokens[i+1].Tokens[EDate].Str)
					e.Sent = e.Sent.UTC()

					e.Subject, err = dec.DecodeHeader(tokens[i+1].Tokens[ESubject].Str)
					if err != nil {
						return nil, err
					}

					for _, a := range []struct {
						dest  *map[string]string
						pos   uint8
						debug string
					}{
						{&e.From, EFrom, "FROM"},
						{&e.ReplyTo, EReplyTo, "REPLYTO"},
						{&e.To, ETo, "TO"},
						{&e.CC, ECC, "CC"},
						{&e.BCC, EBCC, "BCC"},
					} {
						if tokens[i+1].Tokens[EFrom].Type != TNil {
							if err = CheckType(tokens[i+1].Tokens[a.pos], []TType{TNil, TContainer}, "for ENVELOPE[%d]", a.pos); err != nil {
								return nil, err
							}
							*a.dest = make(map[string]string, len(tokens[i+1].Tokens[EFrom].Tokens))
							for i, t := range tokens[i+1].Tokens[a.pos].Tokens {
								if err = CheckType(t.Tokens[EEName], []TType{TQuoted, TNil}, "for %s[%d][%d]", a.debug, i, EEName); err != nil {
									return nil, err
								}
								if err = CheckType(t.Tokens[EEMailbox], []TType{TQuoted, TNil}, "for %s[%d][%d]", a.debug, i, EEMailbox); err != nil {
									return nil, err
								}
								if err = CheckType(t.Tokens[EEHost], []TType{TQuoted}, "for %s[%d][%d]", a.debug, i, EEHost); err != nil {
									return nil, err
								}

								name, err := dec.DecodeHeader(t.Tokens[EEName].Str)
								if err != nil {
									return nil, err
								}

								if t.Tokens[EEMailbox].Type == TNil {
									if Verbose {
										log(d.ConnNum, Brown("email address has no mailbox name (probably not a real email), skipping"))
									}
									continue RecordsL
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

					e.MessageID = tokens[i+1].Tokens[EMessageID].Str

					skip++
				case "UID":
					if err = CheckType(tokens[i+1], []TType{TNumber}, "after UID"); err != nil {
						return nil, err
					}
					e.UID = tokens[i+1].Num
					skip++
				}
			}
		}

		emails[e.UID] = e
	}

	return
}

// Token is a fetch response token (e.g. a number, or a quoted section, or a container, etc.)
type Token struct {
	Depth  int
	Type   TType
	Str    string
	Num    int
	Bytes  []byte
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

const TimeFormat = "02-Jan-2006 15:04:05 -0700"

type tokenContainer *[]*Token

// ParseFetchResponse parses a response from a FETCH command into tokens
func (d *Dialer) ParseFetchResponse(r []byte) (records [][]*Token, err error) {
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
			return nil, fmt.Errorf("Unable to parse Fetch line %#v", string(r[:d.GetStrtokI()]))
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
					Depth: depth,
					Type:  currentToken,
					Str:   RemoveSlashes.Replace(string(r[tokenStart : tokenEnd+1])),
				}
			case TLiteral:
				s := string(r[tokenStart : tokenEnd+1])
				num, err := strconv.Atoi(s)
				if err == nil {
					t = &Token{
						Depth: depth,
						Type:  TNumber,
						Num:   num,
					}
				} else {
					if s == "NIL" {
						t = &Token{
							Depth: depth,
							Type:  TNil,
						}
					} else {
						t = &Token{
							Depth: depth,
							Type:  TLiteral,
							Str:   s,
						}
					}
				}
			case TAtom:
				t = &Token{
					Depth: depth,
					Type:  currentToken,
					Str:   string(r[tokenStart : tokenEnd+1]),
				}
			case TContainer:
				t = &Token{
					Depth:  depth,
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
				case IsLiteral(b):
				default:
					tokenEnd = i - 1
					pushToken()
				}
			case TAtom:
				switch {
				case unicode.IsDigit(rune(b)):
				default:
					tokenEnd = i - 1
					size, err := strconv.Atoi(string(r[tokenStart : tokenEnd+1]))
					if err != nil {
						return nil, err
					}
					tokenStart = tokenEnd + 2 + len(nl)
					tokenEnd = tokenStart + size - len(nl) - 1
					i += size
					pushToken()
				}
			}

			switch currentToken {
			case TUnset:
				switch {
				case b == '"':
					currentToken = TQuoted
					tokenStart = i + 1
				case IsLiteral(b):
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
func IsLiteral(b byte) bool {
	switch {
	case unicode.IsDigit(rune(b)),
		unicode.IsLetter(rune(b)),
		b == '\\',
		b == '.',
		b == '[',
		b == ']':
		return true
	}
	return false
}

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
		return fmt.Sprintf("(%s %#v)", tokenType, t.Str)
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
func CheckType(token *Token, acceptableTypes []TType, loc string, v ...interface{}) (err error) {
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
			if i != i {
				types += "|"
			}
			types += GetTokenName(a)
		}
		err = fmt.Errorf("expected %s token %s, got %+v", types, fmt.Sprintf(loc, v...), token)
	}

	return err
}
