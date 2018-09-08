/*
 * File: c:\Users\Brian Leishman\go\src\github.com\BrianLeishman\imap\main.go
 * Project: c:\Users\Brian Leishman\go\src\github.com\BrianLeishman\imap
 * Created Date: Friday September 7th 2018
 * Author: Brian Leishman
 * -----
 * Last Modified: Sat Sep 08 2018
 * Modified By: Brian Leishman
 * -----
 * Copyright (c) 2018 Stumpyinc, LLC
 */

package main

import (
	"bufio"
	"crypto/rand"
	"crypto/tls"
	"fmt"
	"log"
	"math/big"
	"strconv"
	"strings"
	"time"
)

// Dialer is that
type Dialer struct {
	conn *tls.Conn
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
func (d *Dialer) Exec(command string) (response []byte, err error) {
	tag := fmt.Sprintf("%X", bid2())

	c := fmt.Sprintf("%s %s\r\n", tag, command)

	log.Println("->", strings.TrimSpace(c))

	_, err = d.conn.Write([]byte(c))
	if err != nil {
		return
	}

	r := bufio.NewReader(d.conn)

	response = make([]byte, 0)
	for {
		var line []byte
		line, _, err = r.ReadLine()
		if err != nil {
			return
		}
		response = append(response, line...)
		response = append(response, '\n')
		if string(line[:16]) == tag {
			if string(line[17:19]) != "OK" {
				err = fmt.Errorf("imap command failed: %s", line[20:])
				return
			}

			break
		}
	}

	log.Println("<-", strings.TrimSpace(string(response)))

	return
}

// Login attempts to login
func (d *Dialer) Login(username string, password string) (err error) {
	_, err = d.Exec(fmt.Sprintf("LOGIN \"%s\" \"%s\"", username, password))
	return
}

// petite keys ( ͡° ͜ʖ ͡°)
func bid2() (b []byte) {
	t := time.Now().UnixNano()
	b = make([]byte, 8)

	nBig, _ := rand.Int(rand.Reader, big.NewInt(0xff))

	b[0] = byte((t >> 070) & 0xff)
	b[1] = byte((t >> 060) & 0xff)
	b[2] = byte((t >> 050) & 0xff)
	b[3] = byte((t >> 040) & 0xff)
	b[4] = byte((t >> 030) & 0xff)
	b[5] = byte((t >> 020) & 0xff)
	b[6] = byte((t >> 010) & 0xff)
	b[7] = byte(int(nBig.Int64() & 0xf0))

	return
}

func main() {

	imapDialer, err := NewIMAP("swickyeets@gmail.com", "armoire deniable alarm unmapped ferry", "imap.gmail.com", 993)
	defer imapDialer.Close()
	if err != nil {
		log.Fatalln(err)
	} else {
		imapDialer.Exec(`EXAMINE "INBOX"`)
	}

}
