package imap

import (
	"crypto/rand"
	"math/big"
	"time"
)

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
