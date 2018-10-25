package imap

import "bytes"

// This strtok implementation is supposed to resemble the PHP function,
// except that this will return "" if it couldn't find something instead of `false`
// since Go can't return mixed types, and we want to keep the ability of using this function
// in successes in conditions

// StrtokInit starts the strtok sequence
func (d *Dialer) StrtokInit(b []byte, delims []byte) []byte {
	d.strtokI = 0
	d.strtokBytes = b
	return d.Strtok(delims)
}

// Strtok returns the next "token" in the sequence with the given delimeters
func (d *Dialer) Strtok(delims []byte) []byte {
	start := d.strtokI
	for d.strtokI < len(d.strtokBytes) {
		if bytes.Contains(delims, d.strtokBytes[d.strtokI:d.strtokI+1]) {
			if start == d.strtokI {
				start++
			} else {
				d.strtokI++
				return d.strtokBytes[start : d.strtokI-1]
			}
		}
		d.strtokI++
	}

	return d.strtokBytes[start:]
}

// GetStrtokI returns the current position of the tokenizer
func (d *Dialer) GetStrtokI() int {
	return d.strtokI
}
