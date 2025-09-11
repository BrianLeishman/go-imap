package imap

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"fmt"
	"io"
	"mime"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"net"

	retry "github.com/StirlingMarketingGroup/go-retry"
	"github.com/davecgh/go-spew/spew"
	humanize "github.com/dustin/go-humanize"
	"github.com/jhillyerd/enmime"
	"github.com/logrusorgru/aurora"
	"github.com/rs/xid"
	"github.com/sqs/go-xoauth2"
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

// DialTimeout defines how long to wait when establishing a new connection.
// Zero means no timeout.
var DialTimeout time.Duration

// CommandTimeout defines how long to wait for a command to complete.
// Zero means no timeout.
var CommandTimeout time.Duration

// TLSSkipVerify disables certificate verification when establishing new
// connections. Use with caution; skipping verification exposes the
// connection to man-in-the-middle attacks.
var TLSSkipVerify bool

var lastResp string

// Dialer is basically an IMAP connection
type Dialer struct {
    conn      *tls.Conn
    Folder    string
    ReadOnly  bool
    Username  string
    Password  string
    Host      string
    Port      int
    Connected bool
    ConnNum   int
    state     int
    stateMu   sync.Mutex
    idleStop  chan struct{}
    idleDone  chan struct{}
    // useXOAUTH2 indicates whether XOAUTH2 authentication should be used
    // on (re)connection instead of LOGIN. It is set by NewWithOAuth2.
    useXOAUTH2 bool
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

const (
	StateDisconnected = iota
	StateConnected
	StateSelected
	StateIdlePending
	StateIdling
	StateStoppingIdle
)

// Attachment is an Email attachment
type Attachment struct {
	Name     string
	MimeType string
	Content  []byte
}

type FlagSet int

const (
	FlagUnset FlagSet = iota
	FlagAdd
	FlagRemove
)

type Flags struct {
	Seen     FlagSet
	Answered FlagSet
	Flagged  FlagSet
	Deleted  FlagSet
	Draft    FlagSet
	Keywords map[string]bool
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

func dialHost(host string, port int) (*tls.Conn, error) {
	dialer := &net.Dialer{Timeout: DialTimeout}
	var cfg *tls.Config
	if TLSSkipVerify {
		cfg = &tls.Config{InsecureSkipVerify: true}
	}
	return tls.DialWithDialer(dialer, "tcp", host+":"+strconv.Itoa(port), cfg)
}

// NewWithOAuth2 makes a new imap with OAuth2
func NewWithOAuth2(username string, accessToken string, host string, port int) (d *Dialer, err error) {
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
		conn, err = dialHost(host, port)
		if err != nil {
			if Verbose {
				log(connNum, "", aurora.Red(aurora.Bold(fmt.Sprintf("failed to connect: %s", err))))
			}
			return err
		}
        d = &Dialer{
            conn:      conn,
            Username:  username,
            Password:  accessToken,
            Host:      host,
            Port:      port,
            Connected: true,
            ConnNum:   connNum,
            useXOAUTH2: true,
        }

		return d.Authenticate(username, accessToken)
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
			if d != nil && d.conn != nil {
				d.conn.Close()
			}
		}
		return nil, err
	}

	return
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
		conn, err = dialHost(host, port)
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
            useXOAUTH2: false,
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
			if d != nil && d.conn != nil {
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
    if d.useXOAUTH2 {
        d2, err = NewWithOAuth2(d.Username, d.Password, d.Host, d.Port)
    } else {
        d2, err = New(d.Username, d.Password, d.Host, d.Port)
    }
    // d2.Verbose = d1.Verbose
    if d.Folder != "" {
        if d.ReadOnly {
            err = d2.ExamineFolder(d.Folder)
        } else {
			err = d2.SelectFolder(d.Folder)
		}
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

    conn, err := dialHost(d.Host, d.Port)
    if err != nil {
        return fmt.Errorf("imap reconnect dial: %s", err)
    }
    d.conn = conn
    d.Connected = true

    // Re-authenticate using the original method
    if d.useXOAUTH2 {
        if err := d.Authenticate(d.Username, d.Password); err != nil {
            // Best effort cleanup on failure
            d.conn.Close()
            d.Connected = false
            return fmt.Errorf("imap reconnect auth xoauth2: %s", err)
        }
    } else {
        if err := d.Login(d.Username, d.Password); err != nil {
            d.conn.Close()
            d.Connected = false
            return fmt.Errorf("imap reconnect login: %s", err)
        }
    }

    // Restore selected folder state if any
    if d.Folder != "" {
        if d.ReadOnly {
            if err := d.ExamineFolder(d.Folder); err != nil {
                return fmt.Errorf("imap reconnect examine: %s", err)
            }
        } else {
            if err := d.SelectFolder(d.Folder); err != nil {
                return fmt.Errorf("imap reconnect select: %s", err)
            }
        }
    }

    return nil
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

// Regex to find the start of each "* N FETCH" line in a potentially multi-line response.
// (?m) enables multi-line mode, so ^ matches the start of a line.
var fetchLineStartRE = regexp.MustCompile(`(?m)^\* \d+ FETCH`)

// Exec executes the command on the imap connection
func (d *Dialer) Exec(command string, buildResponse bool, retryCount int, processLine func(line []byte) error) (response string, err error) {
	var resp strings.Builder
	err = retry.Retry(func() (err error) {
		tag := []byte(fmt.Sprintf("%X", xid.New()))

		if CommandTimeout != 0 {
			d.conn.SetDeadline(time.Now().Add(CommandTimeout))
			defer d.conn.SetDeadline(time.Time{})
		}

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

			// XID project is returning 40-byte tags. The code was originally hardcoded 16 digits.
			taglen := len(tag)
			oklen := 3
			if len(line) >= taglen+oklen && bytes.Equal(line[:taglen], tag) {
				if !bytes.Equal(line[taglen+1:taglen+oklen], []byte("OK")) {
					err = fmt.Errorf("imap command failed: %s", line[taglen+oklen+1:])
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

func (d *Dialer) Authenticate(user string, accessToken string) (err error) {
	b64 := xoauth2.XOAuth2String(user, accessToken)
	_, err = d.Exec(fmt.Sprintf("AUTHENTICATE XOAUTH2 %s", b64), false, RetryCount, nil)
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

func (d *Dialer) runIdleEvent(data []byte, handler *IdleHandler) error {
	index := 0
	event := ""
	if _, err := fmt.Sscanf(string(data), "%d %s", &index, &event); err != nil {
		return fmt.Errorf("invalid IDLE event format: %s", data)
	}
	switch event {
	case IdleEventExists:
		if handler.OnExists != nil {
			go handler.OnExists(ExistsEvent{MessageIndex: index})
		}
	case IdleEventExpunge:
		if handler.OnExpunge != nil {
			go handler.OnExpunge(ExpungeEvent{MessageIndex: index})
		}
	case IdleEventFetch:
		if handler.OnFetch == nil {
			return nil
		}
		str := string(data)
		re := regexp.MustCompile(`(?i)^(\d+)\s+FETCH\s+\(([^)]*FLAGS\s*\(([^)]*)\)[^)]*)`)
		matches := re.FindStringSubmatch(str)
		if len(matches) == 4 {
			messageIndex, _ := strconv.Atoi(matches[1])
			uid, _ := strconv.Atoi(matches[2])
			flags := strings.FieldsFunc(strings.ReplaceAll(matches[3], `\`, ""), func(r rune) bool {
				return unicode.IsSpace(r) || r == ','
			})
			go handler.OnFetch(FetchEvent{MessageIndex: messageIndex, UID: uint32(uid), Flags: flags})
		} else {
			return fmt.Errorf("invalid FETCH event format: %s", data)
		}
	}

	return nil
}

func (d *Dialer) StartIdle(handler *IdleHandler) error {
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()

		for {
			if !d.Connected {
				if err := d.Reconnect(); err != nil {
					if Verbose {
						log(d.ConnNum, d.Folder, aurora.Red(fmt.Sprintf("StartIdle error with reconnect: %v", err)))
					}
					return
				}
			}
			if err := d.startIdleSingle(handler); err != nil {
				if Verbose {
					log(d.ConnNum, d.Folder, aurora.Red(fmt.Sprintf("StartIdle error: %v", err)))
				}
				return
			}

			select {
			case <-ticker.C:
				d.StopIdle()
			case <-d.idleDone:
				return
			}
		}
	}()

	return nil
}

func (d *Dialer) startIdleSingle(handler *IdleHandler) error {
	if d.State() == StateIdling || d.State() == StateIdlePending {
		return fmt.Errorf("already entering or in IDLE")
	}

	d.setState(StateIdlePending)

	d.idleStop = make(chan struct{})
	d.idleDone = make(chan struct{})
	idleReady := make(chan struct{})

	go func() {
		defer func() {
			close(d.idleStop)
			if d.State() == StateIdling {
				d.setState(StateSelected)
			}
		}()

		_, err := d.Exec("IDLE", true, 0, func(line []byte) error {
			line = []byte(strings.ToUpper(string(line)))
			switch {
			case bytes.HasPrefix(line, []byte("+")):
				d.setState(StateIdling)
				close(idleReady)
				return nil
			case bytes.HasPrefix(line, []byte("* ")):
				strLine := string(line[2:])
				if strings.HasPrefix(strLine, "OK") {
					return nil
				}
				if strings.HasPrefix(strLine, "BYE") {
					d.setState(StateDisconnected)
					d.Close()
					return fmt.Errorf("server sent BYE: %s", line)
				}
				return d.runIdleEvent([]byte(strLine), handler)
			case bytes.HasPrefix(line, []byte("OK ")):
				strLine := string(line[3:])
				if strings.HasPrefix(strLine, "IDLE") {
					d.setState(StateSelected)
				}
				return nil
			}
			return nil
		})

		if err != nil {
			if Verbose {
				log(d.ConnNum, d.Folder, aurora.Red(fmt.Sprintf("IDLE error: %v", err)))
			}
			d.setState(StateDisconnected)
		}
	}()

	select {
	case <-idleReady:
		return nil
	case <-time.After(5 * time.Second):
		d.setState(StateSelected)
		return fmt.Errorf("timeout waiting for + IDLE response")
	}
}

func (d *Dialer) StopIdle() error {
	if d.State() != StateIdling {
		return fmt.Errorf("not in IDLE state")
	}

	if Verbose {
		log(d.ConnNum, d.Folder, aurora.Bold("-> DONE"))
	}
	if _, err := d.conn.Write([]byte("DONE\r\n")); err != nil {
		return fmt.Errorf("failed to send DONE: %v", err)
	}

	d.setState(StateStoppingIdle)
	close(d.idleDone)

	<-d.idleStop
	d.idleDone, d.idleStop = nil, nil
	if d.State() == StateStoppingIdle {
		d.setState(StateSelected)
	}

	return nil
}

func (d *Dialer) setState(s int) {
	d.stateMu.Lock()
	defer d.stateMu.Unlock()
	d.state = s
}

func (d *Dialer) State() int {
	d.stateMu.Lock()
	defer d.stateMu.Unlock()
	return d.state
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

		err = d.ExamineFolder(f)
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
		err = d.ExamineFolder(folder)
		if err != nil {
			return
		}
	}

	return
}

// ExamineFolder selects a folder
func (d *Dialer) ExamineFolder(folder string) (err error) {
	_, err = d.Exec(`EXAMINE "`+AddSlashes.Replace(folder)+`"`, true, RetryCount, nil)
	if err != nil {
		return
	}
	d.Folder = folder
	d.ReadOnly = true
	return nil
}

// SelectFolder selects a folder
func (d *Dialer) SelectFolder(folder string) (err error) {
	_, err = d.Exec(`SELECT "`+AddSlashes.Replace(folder)+`"`, true, RetryCount, nil)
	if err != nil {
		return
	}
	d.Folder = folder
	d.ReadOnly = false
	return nil
}

// Move a read email to a specified folder
func (d *Dialer) MoveEmail(uid int, folder string) (err error) {
	// if we are currently read-only, switch to SELECT for the move-operation
	readOnlyState := d.ReadOnly
	if readOnlyState {
		d.SelectFolder(d.Folder)
	}
	_, err = d.Exec(`UID MOVE `+strconv.Itoa(uid)+` "`+AddSlashes.Replace(folder)+`"`, true, RetryCount, nil)
	if readOnlyState {
		d.ExamineFolder(d.Folder)
	}
	if err != nil {
		return
	}
	d.Folder = folder
	return nil
}

// mark an emai as seen
func (d *Dialer) MarkSeen(uid int) (err error) {
	flags := Flags{
		Seen: FlagAdd,
	}

	readOnlyState := d.ReadOnly
	if readOnlyState {
		d.SelectFolder(d.Folder)
	}
	err = d.SetFlags(uid, flags)
	if readOnlyState {
		d.ExamineFolder(d.Folder)
	}

	return
}

// DeleteEmail marks an email as deleted
func (d *Dialer) DeleteEmail(uid int) (err error) {
	flags := Flags{
		Deleted: FlagAdd,
	}

	readOnlyState := d.ReadOnly
	if readOnlyState {
		if err = d.SelectFolder(d.Folder); err != nil {
			return
		}
	}
	err = d.SetFlags(uid, flags)
	if readOnlyState {
		if e := d.ExamineFolder(d.Folder); e != nil && err == nil {
			err = e
		}
	}

	return
}

// Expunge permanently removes messages marked as deleted in the current folder
func (d *Dialer) Expunge() (err error) {
	readOnlyState := d.ReadOnly
	if readOnlyState {
		if err = d.SelectFolder(d.Folder); err != nil {
			return
		}
	}
	_, err = d.Exec("EXPUNGE", false, RetryCount, nil)
	if readOnlyState {
		if e := d.ExamineFolder(d.Folder); e != nil && err == nil {
			err = e
		}
	}
	return
}

// set system-flags and keywords
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
		d.SelectFolder(d.Folder)
	}
	_, err = d.Exec(query, true, RetryCount, nil)
	if readOnlyState {
		d.ExamineFolder(d.Folder)
	}

	return err
}

// GetUIDs returns the UIDs in the current folder that match the search
func parseUIDSearchResponse(r string) ([]int, error) {
	if idx := strings.Index(r, nl); idx != -1 {
		r = r[:idx]
	}
	fields := strings.Fields(r)
	if len(fields) >= 2 && fields[0] == "*" && fields[1] == "SEARCH" {
		uids := make([]int, 0, len(fields)-2)
		for _, f := range fields[2:] {
			u, err := strconv.Atoi(f)
			if err != nil {
				return nil, err
			}
			uids = append(uids, u)
		}
		return uids, nil
	}
	return nil, fmt.Errorf("invalid response: %q", r)
}

func (d *Dialer) GetUIDs(search string) (uids []int, err error) {
	r, err := d.Exec(`UID SEARCH `+search, true, RetryCount, nil)
	if err != nil {
		return nil, err
	}
	return parseUIDSearchResponse(r)
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
// If no UIDs are given, then everything in the current folder is selected
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
// If no UIDs are given, then everything in the current folder is selected
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
        // Defensively flatten if the record is a single container wrapper.
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
func parseFetchTokens(r string) ([]*Token, error) {
	tokens := make([]*Token, 0)

	currentToken := TUnset
	tokenStart := 0
	tokenEnd := 0
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
				// Still accumulating digits for size, main loop's i++ will advance
			default: // Should be '}'
				tokenEndOfSize := i // Current 'i' is at '}'
				// tokenStart for size was set when '{' was seen. r[tokenStart:tokenEndOfSize] is the size string.
				sizeVal, err := strconv.Atoi(string(r[tokenStart:tokenEndOfSize]))
				if err != nil {
					return nil, fmt.Errorf("TAtom size Atoi failed for '%s': %w", string(r[tokenStart:tokenEndOfSize]), err)
				}

				i++ // Advance 'i' past '}' to the start of actual literal data

				// skip CRLF
				if i < len(r) && r[i] == '\r' {
					i++
				}
				if i < len(r) && r[i] == '\n' {
					i++
				}

				tokenStart = i // tokenStart is now for the literal data itself

				// Defensive boundary checks
				if tokenStart >= len(r) { // Literal data is empty and we're at/past end of buffer
					if sizeVal == 0 { // Correct for {0}
						tokenEnd = tokenStart - 1 // Results in empty string for r[tokenStart:tokenEnd+1]
					} else { // Error: sizeVal > 0 but no data
						return nil, fmt.Errorf("TAtom: literal size %d but tokenStart %d is at/past end of buffer %d", sizeVal, tokenStart, len(r))
					}
				} else if tokenStart+sizeVal > len(r) { // Declared size is too large for available data
					tokenEnd = len(r) - 1 // Taking available data
				} else { // Normal case: sizeVal fits
					tokenEnd = tokenStart + sizeVal - 1
				}

				i = tokenEnd // Move main loop cursor to the end of the literal data
				pushToken()  // Push the TAtom token
			}
		}

		switch currentToken {
		case TUnset: // If no token is being actively parsed
			switch {
			case b == '"':
				currentToken = TQuoted
				tokenStart = i + 1
			case IsLiteral(rune(b)):
				currentToken = TLiteral
				tokenStart = i
			case b == '{': // Start of a new literal
				currentToken = TAtom
				tokenStart = i + 1 // tokenStart for the size digits
			case b == '(':
				currentToken = TContainer
				t := pushToken() // push any pending token before starting container
				depth++
				// Grow container stack if needed
				if depth >= len(container) {
					newContainer := make([]tokenContainer, depth*2)
					copy(newContainer, container)
					container = newContainer
				}
				container[depth] = &t.Tokens
			case b == ')':
				if depth == 0 { // Unmatched ')'
					return nil, fmt.Errorf("unmatched ')' at char %d in %s", i, r)
				}
				pushToken() // push any pending token before closing container
				depth--
			}
		}

	Cont:
		if depth < 0 {
			break
		}
		i++
		if i >= l { // If we've processed all characters or gone past
			if currentToken != TUnset { // Only push if there's a pending token
				tokenEnd = l - 1 // The last character is at index l-1
				pushToken()
			}
		}
	}

	if depth != 0 {
		return nil, fmt.Errorf("mismatched parentheses, depth %d at end of parsing %s", depth, r)
	}

	if len(tokens) == 1 && tokens[0].Type == TContainer {
		tokens = tokens[0].Tokens
	}

	return tokens, nil
}

func (d *Dialer) ParseFetchResponse(responseBody string) (records [][]*Token, err error) {
	records = make([][]*Token, 0)
	trimmedResponseBody := strings.TrimSpace(responseBody)
	if trimmedResponseBody == "" {
		return records, nil
	}

	locs := fetchLineStartRE.FindAllStringIndex(trimmedResponseBody, -1)

	if locs == nil {
		// No FETCH lines found by regex.
		// Try to parse as a single line if it starts with "* ".
		if strings.HasPrefix(trimmedResponseBody, "* ") {
			currentLineToProcess := trimmedResponseBody
			// Standard parsing logic for a single line
			if !strings.HasPrefix(currentLineToProcess, "* ") {
				return nil, fmt.Errorf("unable to parse Fetch line (expected '* ' prefix): %#v", currentLineToProcess)
			}
			rest := currentLineToProcess[2:]
			idx := strings.IndexByte(rest, ' ')
			if idx == -1 {
				return nil, fmt.Errorf("unable to parse Fetch line (no space after seq number): %#v", currentLineToProcess)
			}
			seqNumStr := rest[:idx]
			if _, convErr := strconv.Atoi(seqNumStr); convErr != nil {
				return nil, fmt.Errorf("unable to parse Fetch line (invalid seq num %s): %#v: %w", seqNumStr, currentLineToProcess, convErr)
			}
			rest = strings.TrimSpace(rest[idx+1:])
			if !strings.HasPrefix(rest, "FETCH ") {
				return nil, fmt.Errorf("unable to parse Fetch line (expected 'FETCH ' prefix after seq num): %#v", currentLineToProcess)
			}
			fetchContent := rest[len("FETCH "):]
			tokens, parseErr := parseFetchTokens(fetchContent)
			if parseErr != nil {
				return nil, fmt.Errorf("token parsing failed for line part [%s] from original line [%s]: %w", fetchContent, currentLineToProcess, parseErr)
			}
			records = append(records, tokens)
			return records, nil
		}
		// If not starting with "* " and no FETCH lines found by regex, return empty or error.
		return records, nil
	}

	for i, loc := range locs {
		start := loc[0]
		end := len(trimmedResponseBody)
		if i+1 < len(locs) {
			end = locs[i+1][0]
		}
		line := trimmedResponseBody[start:end]
		currentLineToProcess := strings.TrimSpace(line)

		if len(currentLineToProcess) == 0 {
			continue
		}

		if !strings.HasPrefix(currentLineToProcess, "* ") {
			return nil, fmt.Errorf("unable to parse Fetch line (expected '* ' prefix, regex mismatch?): %#v", currentLineToProcess)
		}
		rest := currentLineToProcess[2:]
		idx := strings.IndexByte(rest, ' ')
		if idx == -1 {
			return nil, fmt.Errorf("unable to parse Fetch line (no space after seq number, regex mismatch?): %#v", currentLineToProcess)
		}

		seqNumStr := rest[:idx]
		if _, convErr := strconv.Atoi(seqNumStr); convErr != nil {
			return nil, fmt.Errorf("unable to parse Fetch line (invalid seq num %s): %#v: %w", seqNumStr, currentLineToProcess, convErr)
		}

		rest = strings.TrimSpace(rest[idx+1:])
		if !strings.HasPrefix(rest, "FETCH ") {
			return nil, fmt.Errorf("unable to parse Fetch line (expected 'FETCH ' prefix after seq num, regex mismatch?): %#v", currentLineToProcess)
		}

		fetchContent := rest[len("FETCH "):]
		tokens, err := parseFetchTokens(fetchContent)
		if err != nil {
			return nil, fmt.Errorf("token parsing failed for line part [%s] from original line [%s]: %w", fetchContent, currentLineToProcess, err)
		}
		records = append(records, tokens)
	}
	return records, nil
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
