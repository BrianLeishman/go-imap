package imap

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"log"
	"strconv"
	"strings"
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

// Exec executes the command on the imap connection
func (d *Dialer) Exec(command string, buildResponse bool, newlinesBetweenLines bool, processLine func(line []byte)) (response []byte, err error) {
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

		if string(line[:16]) == tag {
			if string(line[17:19]) != "OK" {
				err = fmt.Errorf("imap command failed: %s", line[20:])
				return
			}

			break
		}

		if processLine != nil {
			processLine(line)
		}
		if buildResponse || d.Verbose {
			response = append(response, line...)
			if newlinesBetweenLines {
				response = append(response, '\n')
			}
		}
	}

	if d.Verbose {
		log.Println("<-", strings.TrimSpace(string(response)))
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
	_, err = d.Exec(`LIST "" "*"`, false, false, func(line []byte) {
		if nextLineIsFolder {
			folders = append(folders, string(line))
			nextLineIsFolder = false
		} else {
			i := len(line) - 1
			quoted := line[i] == '"'
			if line[i] == '}' {
				nextLineIsFolder = true
				return
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
