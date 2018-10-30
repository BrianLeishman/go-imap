package imap

import "bytes"

// This strtok implementation is supposed to resemble the PHP function,
// except that this will return "" if it couldn't find something instead of `false`
// since Go can't return mixed types, and we want to keep the ability of using this function
// in successes in conditions

// StrtokInit starts the strtok sequence
func (d *Dialer) StrtokInit(b string, delims []byte) string {
	d.strtokI = 0
	d.strtok = b
	return d.Strtok(delims)
}

// Strtok returns the next "token" in the sequence with the given delimeters
func (d *Dialer) Strtok(delims []byte) string {
	start := d.strtokI
	for d.strtokI < len(d.strtok) {
		if bytes.ContainsRune(delims, rune(d.strtok[d.strtokI])) {
			if start == d.strtokI {
				start++
			} else {
				d.strtokI++
				return string(d.strtok[start : d.strtokI-1])
			}
		}
		d.strtokI++
	}

	return string(d.strtok[start:])
}

// GetStrtokI returns the current position of the tokenizer
func (d *Dialer) GetStrtokI() int {
	return d.strtokI
}
