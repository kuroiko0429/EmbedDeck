package main

import (
	"bufio"
	"bytes"
	"io"
)

// readFrame drains an oversized line through its delimiter before returning.
func readFrame(r *bufio.Reader) ([]byte, error) {
	b := make([]byte, 0, 256)
	over := false
	for {
		part, e := r.ReadSlice('\n')
		if len(b)+len(part) > maxFrame+1 {
			over = true
		}
		if !over {
			b = append(b, part...)
		}
		if e == bufio.ErrBufferFull {
			continue
		}
		if e != nil {
			if e == io.EOF && len(b) > 0 {
				return nil, io.ErrUnexpectedEOF
			}
			return nil, e
		}
		if over {
			return nil, errFrame
		}
		b = bytes.TrimSuffix(b, []byte{'\n'})
		b = bytes.TrimSuffix(b, []byte{'\r'})
		if len(b) > maxFrame {
			return nil, errFrame
		}
		return b, nil
	}
}
