package imap

import (
	"context"
	"crypto/tls"
	"log/slog"
	"net"
	"time"
)

// ContextDialer matches net.Dialer.DialContext and allows injecting a custom
// dialer (e.g. SOCKS proxy).
type ContextDialer interface {
	DialContext(ctx context.Context, network, address string) (net.Conn, error)
}

// Options configures a new Client. The zero value is not usable — Host and
// Auth are required. Everything else has a sensible default.
type Options struct {
	// Host is the IMAP server hostname. Required.
	Host string

	// Port is the IMAP server port. Defaults to 993 (IMAPS).
	Port int

	// Auth is the authentication method. Required.
	Auth Authenticator

	// DialTimeout bounds how long Dial waits to establish the TCP/TLS
	// connection. Zero means no timeout.
	DialTimeout time.Duration

	// CommandTimeout is the per-command deadline.
	//   positive: use this duration for every command
	//   zero:     fall back to the package-level CommandTimeout
	//   negative: explicitly disable the deadline for this client
	CommandTimeout time.Duration

	// RetryCount is the number of times to retry a failed command.
	//   positive: retry up to this many times
	//   zero:     fall back to the package-level RetryCount
	//   negative: explicitly disable retries for this client
	RetryCount int

	// TLSConfig overrides the default TLS configuration. When nil, a secure
	// default is used. Dial will set ServerName to Host if ServerName is
	// empty so that hostname verification works out of the box. The caller's
	// config is not mutated.
	TLSConfig *tls.Config

	// Dialer overrides the default *net.Dialer used to establish the TCP
	// connection. Useful for SOCKS proxies or custom network setups.
	Dialer ContextDialer

	// Logger is the slog logger used for all library output. When nil, the
	// package-level logger is used (see SetSlogLogger).
	Logger *slog.Logger
}

// Authenticator performs IMAP authentication after the TLS connection is
// established.
type Authenticator interface {
	authenticate(ctx context.Context, c *Client) error
}

// PasswordAuth authenticates using the IMAP LOGIN command.
type PasswordAuth struct {
	Username string
	Password string
}

func (a PasswordAuth) authenticate(ctx context.Context, c *Client) error {
	return c.Login(ctx, a.Username, a.Password)
}

// XOAuth2 authenticates using the XOAUTH2 SASL mechanism.
type XOAuth2 struct {
	Username    string
	AccessToken string
}

func (a XOAuth2) authenticate(ctx context.Context, c *Client) error {
	return c.Authenticate(ctx, a.Username, a.AccessToken)
}
