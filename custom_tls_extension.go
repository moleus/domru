package main

import (
	tls "github.com/refraction-networking/utls"
	"io"
)

const FakeEncryptThenMac uint16 = 0x0016

type FakeEncryptThenMacExtension struct {
	*tls.GenericExtension
}

func (e *FakeEncryptThenMacExtension) Len() int {
	return 4
}

func (e *FakeEncryptThenMacExtension) Read(b []byte) (n int, err error) {
	if len(b) < e.Len() {
		return 0, io.ErrShortBuffer
	}
	b[0] = byte(FakeEncryptThenMac >> 8)
	b[1] = byte(FakeEncryptThenMac)
	// The length is 0
	return e.Len(), io.EOF
}
