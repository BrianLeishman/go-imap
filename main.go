package imap

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"log"
	"mime"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/jhillyerd/enmime"
)

// AddSlashes adds slashes to double quotes
var AddSlashes = strings.NewReplacer(`"`, `\"`)

// RemoveSlashes removes slashes before double quotes
var RemoveSlashes = strings.NewReplacer(`\"`, `"`)

// Dialer is that
type Dialer struct {
	conn    *tls.Conn
	Folder  string
	Verbose bool
}

// NewIMAP makes a new imap
func NewIMAP(username string, password string, host string, port int) (d *Dialer, err error) {
	var conn *tls.Conn
	conn, err = tls.Dial("tcp", host+":"+strconv.Itoa(port), nil)
	if err != nil {
		return
	}
	d = &Dialer{conn: conn}

	err = d.Login(username, password)

	return
}

// Close closes the imap connection
func (d *Dialer) Close() {
	d.conn.Close()
}

const nl = "\r\n"

// Exec executes the command on the imap connection
func (d *Dialer) Exec(command string, buildResponse bool, newlinesBetweenLines bool, processLine func(line []byte) error) (response []byte, err error) {
	tag := fmt.Sprintf("%X", bid2())

	c := fmt.Sprintf("%s %s\r\n", tag, command)

	if d.Verbose {
		log.Println("->", strings.TrimSpace(c))
	}

	_, err = d.conn.Write([]byte(c))
	if err != nil {
		return
	}

	r := bufio.NewReader(d.conn)

	if buildResponse {
		response = make([]byte, 0)
	}
	for {
		var line []byte
		line, _, err = r.ReadLine()
		if err != nil {
			return
		}

		if d.Verbose {
			log.Println("<-", string(line))
		}

		if string(line[:16]) == tag {
			if string(line[17:19]) != "OK" {
				err = fmt.Errorf("imap command failed: %s", line[20:])
				return
			}
			break
		}

		if processLine != nil {
			if err = processLine(line); err != nil {
				return nil, err
			}
		}
		if buildResponse {
			response = append(response, line...)
			if newlinesBetweenLines {
				response = append(response, nl...)
			}
		}
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
	r, err := d.Exec(`UID SEARCH ALL`, true, false, nil)
	if err != nil {
		return nil, err
	}
	if string(StrtokInit(r, t)) == "*" && string(Strtok(t)) == "SEARCH" {
		for {
			uid := string(Strtok(t))
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

// Email is... an email
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
	EESR
	EEMailbox
	EEHost
)

// GetEmails returns email with their bodies for the given UIDs in the current folder.
// If no UIDs are given, they everything in the current folder is selected
func (d *Dialer) GetEmails(uids ...int) (emails map[int]*Email, err error) {
	emails, uidsStr, err := d.GetOverviews(uids...)

	r, err := d.Exec("UID FETCH "+uidsStr+" BODY.PEEK[]", true, true, nil)
	if err != nil {
		return nil, err
	}

	records, err := ParseFetchResponse(r)
	if err != nil {
		return nil, err
	}

	for _, tks := range records {
		e := &Email{}
		skip := 0
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

				env, err := enmime.ReadEnvelope(r)
				if err != nil {
					return nil, err
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
				if CheckType(tks[i+1], []TType{TNumber}, "after UID") != nil {
					return nil, err
				}
				e.UID = tks[i+1].Num
				skip++
			}
		}

		emails[e.UID].Subject = e.Subject
		emails[e.UID].From = e.From
		emails[e.UID].ReplyTo = e.ReplyTo
		emails[e.UID].To = e.To
		emails[e.UID].CC = e.CC
		emails[e.UID].BCC = e.BCC
		emails[e.UID].Text = e.Text
		emails[e.UID].HTML = e.HTML
	}

	return
}

// GetOverviews returns emails without bodies for the given UIDs in the current folder.
// If no UIDs are given, they everything in the current folder is selected
func (d *Dialer) GetOverviews(uids ...int) (emails map[int]*Email, uidsStr string, err error) {
	_uids := strings.Builder{}
	if len(uids) == 0 {
		_uids.WriteString("1:*")
	} else {
		for i, u := range uids {
			if i != 0 {
				_uids.WriteByte(',')
			}
			_uids.WriteString(strconv.Itoa(u))
		}
	}

	uidsStr = _uids.String()

	emails = make(map[int]*Email, len(uids))
	dec := new(mime.WordDecoder)

	r, err := d.Exec("UID FETCH "+uidsStr+" ALL", true, true, nil)
	if err != nil {
		return nil, "", err
	}

	records, err := ParseFetchResponse(r)
	if err != nil {
		return nil, "", err
	}

	for _, tokens := range records {
		e := &Email{}
		skip := 0
		for i, t := range tokens {
			if skip > 0 {
				skip--
				continue
			}
			if t.Depth == 0 {
				if t.Type != TLiteral {
					log.Fatalf("Expected literal token, got %#v\n", t)
				}
				switch t.Str {
				case "FLAGS":
					if CheckType(tokens[i+1], []TType{TContainer}, "after FLAGS") != nil {
						return nil, "", err
					}
					e.Flags = make([]string, len(tokens[i+1].Tokens))
					for i, t := range tokens[i].Tokens {
						if CheckType(t, []TType{TLiteral}, "for FLAGS[%d]", i) != nil {
							return nil, "", err
						}
						e.Flags[i] = t.Str
					}
					skip++
				case "INTERNALDATE":
					if CheckType(tokens[i+1], []TType{TQuoted}, "after INTERNALDATE") != nil {
						return nil, "", err
					}
					e.Received, err = time.Parse(TimeFormat, tokens[i+1].Str)
					if err != nil {
						return nil, "", err
					}
					e.Received = e.Received.UTC()
					skip++
				case "RFC822.SIZE":
					if CheckType(tokens[i+1], []TType{TNumber}, "after RFC822.SIZE") != nil {
						return nil, "", err
					}
					e.Size = uint64(tokens[i+1].Num)
					skip++
				case "ENVELOPE":
					if CheckType(tokens[i+1], []TType{TContainer}, "after ENVELOPE") != nil {
						return nil, "", err
					}
					if CheckType(tokens[i+1].Tokens[EDate], []TType{TQuoted}, "for ENVELOPE[%d]", EDate) != nil {
						return nil, "", err
					}
					if CheckType(tokens[i+1].Tokens[ESubject], []TType{TQuoted}, "for ENVELOPE[%d]", ESubject) != nil {
						return nil, "", err
					}

					e.Sent, err = time.Parse("Mon, _2 Jan 2006 15:04:05 -0700", tokens[i+1].Tokens[EDate].Str)
					if err != nil {
						return nil, "", err
					}
					e.Sent = e.Sent.UTC()

					e.Subject, err = dec.DecodeHeader(tokens[i+1].Tokens[ESubject].Str)
					if err != nil {
						return nil, "", err
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
							if CheckType(tokens[i+1].Tokens[a.pos], []TType{TNil, TContainer}, "for ENVELOPE[%d]", a.pos) != nil {
								return nil, "", err
							}
							*a.dest = make(map[string]string, len(tokens[i+1].Tokens[EFrom].Tokens))
							for i, t := range tokens[i+1].Tokens[a.pos].Tokens {
								if CheckType(t.Tokens[EEName], []TType{TQuoted}, "for %s[%d][%d]", a.debug, i, EEName) != nil {
									return nil, "", err
								}
								if CheckType(t.Tokens[EEMailbox], []TType{TQuoted}, "for %s[%d][%d]", a.debug, i, EEMailbox) != nil {
									return nil, "", err
								}
								if CheckType(t.Tokens[EEHost], []TType{TQuoted}, "for %s[%d][%d]", a.debug, i, EEHost) != nil {
									return nil, "", err
								}

								name, err := dec.DecodeHeader(t.Tokens[EEName].Str)
								if err != nil {
									return nil, "", err
								}

								host, err := dec.DecodeHeader(t.Tokens[EEHost].Str)
								if err != nil {
									return nil, "", err
								}

								mailbox, err := dec.DecodeHeader(t.Tokens[EEMailbox].Str)
								if err != nil {
									return nil, "", err
								}

								(*a.dest)[strings.ToLower(mailbox+"@"+host)] = name
							}
						}
					}

					e.MessageID = tokens[i+1].Tokens[EMessageID].Str

					skip++
				case "UID":
					if CheckType(tokens[i+1], []TType{TNumber}, "after UID") != nil {
						return nil, "", err
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
func ParseFetchResponse(r []byte) (records [][]*Token, err error) {
	records = make([][]*Token, 0)
	for {
		t := []byte{' ', '\r', '\n'}
		ok := false
		if string(StrtokInit(r, t)) == "*" {
			if _, err := strconv.Atoi(string(Strtok(t))); err == nil && string(Strtok(t)) == "FETCH" {
				ok = true
			}
		}

		if !ok {
			return nil, fmt.Errorf("Unable to parse Fetch line %#v", string(r[:GetStrtokI()]))
		}

		tokens := make([]*Token, 0)
		r = r[GetStrtokI()+1:]

		currentToken := TUnset
		tokenStart := 0
		tokenEnd := 0
		escaped := false
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
				if !escaped {
					switch b {
					case '"':
						tokenEnd = i - 1
						pushToken()
						goto Cont
					case '\'':
						escaped = true
					}
				} else {
					escaped = false
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
		err = fmt.Errorf("Expected %s token %s, got %#v", 0, fmt.Sprintf(loc, v...), token)
	}

	return
}
