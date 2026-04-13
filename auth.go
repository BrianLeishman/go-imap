package imap

import (
	"context"
	"fmt"

	"github.com/sqs/go-xoauth2"
)

// Authenticate performs XOAUTH2 authentication using an access token.
func (d *Client) Authenticate(ctx context.Context, user string, accessToken string) error {
	b64 := xoauth2.XOAuth2String(user, accessToken)
	// Don't retry authentication - auth failures should not trigger reconnection
	_, err := d.Exec(ctx, fmt.Sprintf("AUTHENTICATE XOAUTH2 %s", b64), false, 0, nil)
	return err
}

// Login performs LOGIN authentication using username and password.
func (d *Client) Login(ctx context.Context, username string, password string) error {
	// Don't retry authentication - auth failures should not trigger reconnection
	_, err := d.Exec(ctx, fmt.Sprintf(`LOGIN "%s" "%s"`, addSlashes.Replace(username), addSlashes.Replace(password)), false, 0, nil)
	return err
}
