package imap

import "context"

// ctx is the default context used by tests. Tests that need cancellation or
// timeouts should construct their own context inline.
var ctx = context.Background()
