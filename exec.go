package imap

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	retry "github.com/StirlingMarketingGroup/go-retry"
	"github.com/rs/xid"
)

// Exec executes an IMAP command with retry logic and response building
func (d *Dialer) Exec(command string, buildResponse bool, retryCount int, processLine func(line []byte) error) (response string, err error) {
	var resp strings.Builder
	err = retry.Retry(func() (err error) {
		tag := []byte(strings.ToUpper(xid.New().String()))

		if CommandTimeout != 0 {
			_ = d.conn.SetDeadline(time.Now().Add(CommandTimeout))
			defer func() { _ = d.conn.SetDeadline(time.Time{}) }()
		}

		c := fmt.Sprintf("%s %s\r\n", tag, command)

		if Verbose {
			sanitized := strings.ReplaceAll(strings.TrimSpace(c), fmt.Sprintf(`"%s"`, d.Password), `"****"`)
			debugLog(d.ConnNum, d.Folder, "sending command", "command", sanitized)
		}

		_, err = d.conn.Write([]byte(c))
		if err != nil {
			return err
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
						return err
					}

					buf := make([]byte, n)
					_, err = io.ReadFull(r, buf)
					if err != nil {
						return err
					}
					line = append(line, buf...)

					buf, err = r.ReadBytes('\n')
					if err != nil {
						return err
					}
					line = append(line, buf...)

					continue
				}
				break
			}

			if Verbose && !SkipResponses {
				debugLog(d.ConnNum, d.Folder, "server response", "response", string(dropNl(line)))
			}

			// if strings.Contains(string(line), "--00000000000030095105741e7f1f") {
			// 	f, _ := ioutil.TempFile("", "")
			// 	ioutil.WriteFile(f.Name(), line, 0777)
			// 	fmt.Println(f.Name())
			// }

			// XID tags are 20 uppercase base32hex characters (0-9, A-V).
			taglen := len(tag)
			oklen := 3
			if len(line) >= taglen+oklen && bytes.Equal(line[:taglen], tag) {
				if !bytes.Equal(line[taglen+1:taglen+oklen], []byte("OK")) {
					err = fmt.Errorf("imap command failed: %s", line[taglen+oklen+1:])
					return err
				}
				break
			}

			if processLine != nil {
				if err = processLine(line); err != nil {
					return err
				}
			}
			if buildResponse {
				resp.Write(line)
			}
		}
		return err
	}, retryCount, func(err error) error {
		if Verbose {
			warnLog(d.ConnNum, d.Folder, "command failed, closing connection", "error", err)
		}
		_ = d.Close()
		return nil
	}, func() error {
		return d.Reconnect()
	})
	if err != nil {
		errorLog(d.ConnNum, d.Folder, "command retries exhausted", "error", err)
		return "", err
	}

	if buildResponse {
		if resp.Len() != 0 {
			lastResp = resp.String()
			return lastResp, nil
		}
		return "", nil
	}
	return response, err
}
