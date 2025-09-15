package imap

import (
	"fmt"

	"github.com/sqs/go-xoauth2"
)

// Authenticate performs XOAUTH2 authentication using an access token
func (d *Dialer) Authenticate(user string, accessToken string) (err error) {
	b64 := xoauth2.XOAuth2String(user, accessToken)
	// Don't retry authentication - auth failures should not trigger reconnection
	_, err = d.Exec(fmt.Sprintf("AUTHENTICATE XOAUTH2 %s", b64), false, 0, nil)
	return err
}

// Login performs LOGIN authentication using username and password
func (d *Dialer) Login(username string, password string) (err error) {
	// Don't retry authentication - auth failures should not trigger reconnection
	_, err = d.Exec(fmt.Sprintf(`LOGIN "%s" "%s"`, AddSlashes.Replace(username), AddSlashes.Replace(password)), false, 0, nil)
	return err
}
