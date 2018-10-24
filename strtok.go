package imap

import "bytes"

// This strtok implementation is supposed to resemble the PHP function,
// except that this will return "" if it couldn't find something instead of `false`
// since Go can't return mixed types, and we want to keep the ability of using this function
// in successes in conditions

var strtokI int
var strtokBytes []byte

// StrtokInit starts the strtok sequence
func StrtokInit(b []byte, delims []byte) []byte {
	strtokI = 0
	strtokBytes = b
	return Strtok(delims)
}

// Strtok returns the next "token" in the sequence with the given delimeters
func Strtok(delims []byte) []byte {
	start := strtokI
	for strtokI < len(strtokBytes) {
		if bytes.Contains(delims, strtokBytes[strtokI:strtokI+1]) {
			if start == strtokI {
				start++
			} else {
				strtokI++
				return strtokBytes[start : strtokI-1]
			}
		}
		strtokI++
	}

	return strtokBytes[start:]
}

// GetStrtokI returns the current position of the tokenizer
func GetStrtokI() int {
	return strtokI
}
