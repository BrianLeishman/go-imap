package imap

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"strconv"
	"sync"
	"time"
)

var (
	nextConnNum      = 0
	nextConnNumMutex = sync.RWMutex{}
)

// Client represents an IMAP connection.
type Client struct {
	conn      *tls.Conn
	Folder    string
	ReadOnly  bool
	Username  string
	password  string
	Host      string
	Port      int
	Connected bool
	ConnNum   int
	state     int
	stateMu   sync.Mutex
	idleStop  chan struct{}
	idleDone  chan struct{}
	// auth is retained so Reconnect can re-authenticate using the same
	// method and credentials the caller originally supplied.
	auth Authenticator
	// opts is the original Dial options (minus Auth, which lives in auth).
	// Retained so Reconnect and Clone can reuse custom TLSConfig, Dialer,
	// timeouts, and retry settings.
	opts Options
}

// credsFromAuth extracts a (username, secret) pair from a known
// Authenticator for log sanitization. Unknown types yield empty strings.
func credsFromAuth(a Authenticator) (string, string) {
	switch v := a.(type) {
	case PasswordAuth:
		return v.Username, v.Password
	case XOAuth2:
		return v.Username, v.AccessToken
	}
	return "", ""
}

// effectiveRetryCount returns the retry count for this client. A positive
// Options.RetryCount is used as-is. A negative value explicitly disables
// retries for this client, regardless of the package-level RetryCount. A
// zero value falls back to the package-level RetryCount.
func (d *Client) effectiveRetryCount() int {
	switch {
	case d.opts.RetryCount > 0:
		return d.opts.RetryCount
	case d.opts.RetryCount < 0:
		return 0
	default:
		return RetryCount
	}
}

// effectiveCommandTimeout returns the per-command deadline for this client.
// A positive Options.CommandTimeout is used as-is. A negative value
// explicitly disables the deadline for this client, regardless of the
// package-level CommandTimeout. A zero value falls back to the package-level
// CommandTimeout.
func (d *Client) effectiveCommandTimeout() time.Duration {
	switch {
	case d.opts.CommandTimeout > 0:
		return d.opts.CommandTimeout
	case d.opts.CommandTimeout < 0:
		return 0
	default:
		return CommandTimeout
	}
}

// SetAuth replaces the client's authenticator. Use this to rotate
// credentials or refresh an OAuth2 access token on a long-lived client;
// subsequent Reconnect and Clone calls will use the new authenticator.
// SetAuth does not itself re-authenticate the open connection.
func (d *Client) SetAuth(a Authenticator) {
	d.auth = a
	// Update stored opts so Clone re-dials with the current credentials.
	d.opts.Auth = a
	// Keep username/password mirrored for diagnostic/log sanitization.
	switch v := a.(type) {
	case PasswordAuth:
		d.Username = v.Username
		d.password = v.Password
	case XOAuth2:
		d.Username = v.Username
		d.password = v.AccessToken
	}
}

// Dialer is a deprecated alias for Client retained so that existing callers
// compile against the v1 API without immediate changes.
//
// Deprecated: use Client. Will be removed in v2.
type Dialer = Client

// dialHost establishes a TLS connection to the IMAP server using the
// supplied options. DialTimeout, when set, bounds both the TCP connect and
// the TLS handshake (matching the prior tls.DialWithDialer behavior).
func dialHost(ctx context.Context, opts Options) (*tls.Conn, error) {
	if opts.DialTimeout > 0 {
		if _, hasDeadline := ctx.Deadline(); !hasDeadline {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, opts.DialTimeout)
			defer cancel()
		}
	}
	address := opts.Host + ":" + strconv.Itoa(opts.Port)

	// Build the TLS config: clone the caller's config so we don't mutate it,
	// and fill in ServerName from Host when the caller didn't set one so
	// hostname verification still works. Match the old tls.DialWithDialer
	// behavior.
	var tlsCfg *tls.Config
	if opts.TLSConfig != nil {
		tlsCfg = opts.TLSConfig.Clone()
	} else {
		tlsCfg = &tls.Config{}
	}
	if tlsCfg.ServerName == "" {
		tlsCfg.ServerName = opts.Host
	}
	if TLSSkipVerify {
		tlsCfg.InsecureSkipVerify = true
	}

	var rawConn net.Conn
	var err error
	if opts.Dialer != nil {
		rawConn, err = opts.Dialer.DialContext(ctx, "tcp", address)
	} else {
		d := &net.Dialer{Timeout: opts.DialTimeout}
		rawConn, err = d.DialContext(ctx, "tcp", address)
	}
	if err != nil {
		return nil, err
	}

	tlsConn := tls.Client(rawConn, tlsCfg)
	if deadline, ok := ctx.Deadline(); ok {
		_ = tlsConn.SetDeadline(deadline)
	}
	if err := tlsConn.HandshakeContext(ctx); err != nil {
		_ = rawConn.Close()
		return nil, err
	}
	_ = tlsConn.SetDeadline(time.Time{})
	return tlsConn, nil
}

// Dial establishes a new IMAP connection using the supplied options and
// authenticates with opts.Auth. The returned Client is ready to issue
// commands.
func Dial(ctx context.Context, opts Options) (*Client, error) {
	if opts.Host == "" {
		return nil, errors.New("imap: Options.Host is required")
	}
	if opts.Auth == nil {
		return nil, errors.New("imap: Options.Auth is required")
	}
	if opts.Port == 0 {
		opts.Port = 993
	}

	nextConnNumMutex.Lock()
	connNum := nextConnNum
	nextConnNum++
	nextConnNumMutex.Unlock()

	// Resolve retry count:
	//   positive = use as-is
	//   negative = explicitly disable (single attempt)
	//   zero     = fall back to the package-level RetryCount
	retryCount := opts.RetryCount
	switch {
	case retryCount > 0:
	case retryCount < 0:
		retryCount = 0
	default:
		retryCount = RetryCount
	}

	// Context-aware retry loop. Unlike retry.Retry which has its own
	// uninterruptible sleep, this loop honors ctx during backoff so
	// cancellation returns promptly.
	var c *Client
	var dialErr error
	for attempt := 0; attempt <= retryCount; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if attempt > 0 {
			debugLog(connNum, "", "retrying connection")
			backoff := min(time.Duration(attempt*250)*time.Millisecond, 3*time.Second)
			t := time.NewTimer(backoff)
			select {
			case <-ctx.Done():
				t.Stop()
				return nil, ctx.Err()
			case <-t.C:
			}
		}

		debugLog(connNum, "", "establishing connection", "host", opts.Host, "port", opts.Port)
		conn, err := dialHost(ctx, opts)
		if err != nil {
			debugLog(connNum, "", "connection attempt failed", "error", err)
			dialErr = err
			continue
		}
		username, password := credsFromAuth(opts.Auth)
		c = &Client{
			conn:      conn,
			Username:  username,
			password:  password,
			Host:      opts.Host,
			Port:      opts.Port,
			Connected: true,
			ConnNum:   connNum,
			auth:      opts.Auth,
			opts:      opts,
		}
		dialErr = nil
		break
	}
	if dialErr != nil || c == nil {
		warnLog(connNum, "", "failed to establish connection", "error", dialErr)
		return nil, dialErr
	}

	if err := opts.Auth.authenticate(ctx, c); err != nil {
		errorLog(connNum, "", "authentication failed", "error", err)
		_ = c.Close()
		return nil, err
	}

	return c, nil
}

// legacyOptions builds Options for the deprecated constructors. Only
// DialTimeout is snapshot at construction time (it's used once during the
// initial dial). CommandTimeout and RetryCount are intentionally left zero
// so that effectiveCommandTimeout and effectiveRetryCount read the
// package-level globals live on every command — preserving the pre-v1
// behavior where mutating imap.CommandTimeout or imap.RetryCount after
// construction affected subsequent operations.
func legacyOptions(host string, port int, auth Authenticator) Options {
	return Options{
		Host:        host,
		Port:        port,
		Auth:        auth,
		DialTimeout: DialTimeout,
	}
}

// New creates a new IMAP connection using LOGIN authentication.
//
// Deprecated: use Dial with PasswordAuth. Will be removed in v2.
func New(username, password, host string, port int) (*Client, error) {
	return Dial(context.Background(), legacyOptions(host, port,
		PasswordAuth{Username: username, Password: password}))
}

// NewWithOAuth2 creates a new IMAP connection using XOAUTH2 authentication.
//
// Deprecated: use Dial with XOAuth2. Will be removed in v2.
func NewWithOAuth2(username, accessToken, host string, port int) (*Client, error) {
	return Dial(context.Background(), legacyOptions(host, port,
		XOAuth2{Username: username, AccessToken: accessToken}))
}

// Clone creates a copy of the client with the same configuration, reusing
// the original Dial options (including TLSConfig, Dialer, and timeouts) and
// authentication method. Runtime changes to Host or Port are honored so
// Clone and Reconnect target the same endpoint.
func (d *Client) Clone(ctx context.Context) (*Client, error) {
	dialOpts := d.opts
	dialOpts.Host = d.Host
	dialOpts.Port = d.Port
	d2, err := Dial(ctx, dialOpts)
	if err != nil {
		return nil, err
	}
	if d.Folder != "" {
		if d.ReadOnly {
			err = d2.ExamineFolder(ctx, d.Folder)
		} else {
			err = d2.SelectFolder(ctx, d.Folder)
		}
		if err != nil {
			return nil, fmt.Errorf("imap clone: %s", err)
		}
	}
	return d2, nil
}

// Close closes the IMAP connection.
func (d *Client) Close() (err error) {
	if d.Connected {
		debugLog(d.ConnNum, d.Folder, "closing connection")
		err = d.conn.Close()
		if err != nil {
			return fmt.Errorf("imap close: %s", err)
		}
		d.Connected = false
	}
	return err
}

// Reconnect closes and reopens the IMAP connection using the client's
// original Dial options, re-authenticates, and restores any selected folder.
func (d *Client) Reconnect(ctx context.Context) error {
	_ = d.Close()
	debugLog(d.ConnNum, d.Folder, "reopening connection")

	// Reuse the original dial options so custom TLSConfig, Dialer, and
	// DialTimeout survive reconnection. Fields on the Client (Host/Port)
	// take precedence in case they were updated after Dial.
	dialOpts := d.opts
	dialOpts.Host = d.Host
	dialOpts.Port = d.Port
	if dialOpts.DialTimeout == 0 {
		dialOpts.DialTimeout = DialTimeout
	}
	conn, err := dialHost(ctx, dialOpts)
	if err != nil {
		return fmt.Errorf("imap reconnect dial: %s", err)
	}
	d.conn = conn
	d.Connected = true

	if d.auth != nil {
		if err := d.auth.authenticate(ctx, d); err != nil {
			_ = d.conn.Close()
			d.Connected = false
			return fmt.Errorf("imap reconnect auth: %s", err)
		}
	}

	if d.Folder != "" {
		if d.ReadOnly {
			if err := d.ExamineFolder(ctx, d.Folder); err != nil {
				return fmt.Errorf("imap reconnect examine: %s", err)
			}
		} else {
			if err := d.SelectFolder(ctx, d.Folder); err != nil {
				return fmt.Errorf("imap reconnect select: %s", err)
			}
		}
	}

	return nil
}
