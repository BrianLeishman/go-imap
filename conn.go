package imap

import (
	"crypto/tls"
	"fmt"
	"net"
	"strconv"
	"sync"

	retry "github.com/StirlingMarketingGroup/go-retry"
	"github.com/logrusorgru/aurora"
)

var (
	nextConnNum      = 0
	nextConnNumMutex = sync.RWMutex{}
)

// Dialer represents an IMAP connection
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

// dialHost establishes a TLS connection to the IMAP server
func dialHost(host string, port int) (*tls.Conn, error) {
	dialer := &net.Dialer{Timeout: DialTimeout}
	var cfg *tls.Config
	if TLSSkipVerify {
		cfg = &tls.Config{InsecureSkipVerify: true}
	}
	return tls.DialWithDialer(dialer, "tcp", host+":"+strconv.Itoa(port), cfg)
}

// NewWithOAuth2 creates a new IMAP connection using OAuth2 authentication
func NewWithOAuth2(username string, accessToken string, host string, port int) (d *Dialer, err error) {
	nextConnNumMutex.RLock()
	connNum := nextConnNum
	nextConnNumMutex.RUnlock()

	nextConnNumMutex.Lock()
	nextConnNum++
	nextConnNumMutex.Unlock()

	// Retry only the connection establishment, not authentication
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
			conn:       conn,
			Username:   username,
			Password:   accessToken,
			Host:       host,
			Port:       port,
			Connected:  true,
			ConnNum:    connNum,
			useXOAUTH2: true,
		}
		return nil
	}, RetryCount, func(err error) error {
		if Verbose {
			log(connNum, "", aurora.Yellow(aurora.Bold("failed to connect, retrying shortly")))
			if d != nil && d.conn != nil {
				_ = d.conn.Close()
			}
		}
		return nil
	}, func() error {
		if Verbose {
			log(connNum, "", aurora.Yellow(aurora.Bold("retrying connection now")))
		}
		return nil
	})
	if err != nil {
		if Verbose {
			log(connNum, "", aurora.Red(aurora.Bold("failed to establish connection")))
			if d != nil && d.conn != nil {
				_ = d.conn.Close()
			}
		}
		return nil, err
	}

	// Authenticate after connection is established - no retry for auth failures
	err = d.Authenticate(username, accessToken)
	if err != nil {
		if Verbose {
			log(connNum, "", aurora.Red(aurora.Bold(fmt.Sprintf("authentication failed: %s", err))))
		}
		_ = d.Close()
		return nil, err
	}

	return d, nil
}

// New creates a new IMAP connection using username/password authentication
func New(username string, password string, host string, port int) (d *Dialer, err error) {
	nextConnNumMutex.RLock()
	connNum := nextConnNum
	nextConnNumMutex.RUnlock()

	nextConnNumMutex.Lock()
	nextConnNum++
	nextConnNumMutex.Unlock()

	// Retry only the connection establishment, not authentication
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
			conn:       conn,
			Username:   username,
			Password:   password,
			Host:       host,
			Port:       port,
			Connected:  true,
			ConnNum:    connNum,
			useXOAUTH2: false,
		}
		return nil
	}, RetryCount, func(err error) error {
		if Verbose {
			log(connNum, "", aurora.Yellow(aurora.Bold("failed to connect, retrying shortly")))
			if d != nil && d.conn != nil {
				_ = d.conn.Close()
			}
		}
		return nil
	}, func() error {
		if Verbose {
			log(connNum, "", aurora.Yellow(aurora.Bold("retrying connection now")))
		}
		return nil
	})
	if err != nil {
		if Verbose {
			log(connNum, "", aurora.Red(aurora.Bold("failed to establish connection")))
			if d != nil && d.conn != nil {
				_ = d.conn.Close()
			}
		}
		return nil, err
	}

	// Authenticate after connection is established - no retry for auth failures
	err = d.Login(username, password)
	if err != nil {
		if Verbose {
			log(connNum, "", aurora.Red(aurora.Bold(fmt.Sprintf("authentication failed: %s", err))))
		}
		_ = d.Close()
		return nil, err
	}

	return d, nil
}

// Clone creates a copy of the dialer with the same configuration
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
	return d2, err
}

// Close closes the IMAP connection
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
	return err
}

// Reconnect closes and reopens the IMAP connection with re-authentication
func (d *Dialer) Reconnect() (err error) {
	_ = d.Close()
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
			_ = d.conn.Close()
			d.Connected = false
			return fmt.Errorf("imap reconnect auth xoauth2: %s", err)
		}
	} else {
		if err := d.Login(d.Username, d.Password); err != nil {
			_ = d.conn.Close()
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
