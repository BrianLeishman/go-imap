package imap

import (
	"strings"
	"time"
)

// String replacers for escaping/unescaping quotes
var (
	AddSlashes    = strings.NewReplacer(`"`, `\"`)
	RemoveSlashes = strings.NewReplacer(`\"`, `"`)
)

// Verbose outputs every command and its response with the IMAP server
var Verbose = false

// SkipResponses skips printing server responses in verbose mode
var SkipResponses = false

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
