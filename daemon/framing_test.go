package main

import (
	"bufio"
	"fmt"
	"strings"
	"testing"
)

func TestFramingByteLimitAndRecovery(t *testing.T) {
	for _, bufferSize := range []int{128, 4096} {
		for _, ending := range []string{"\n", "\r\n"} {
			for _, size := range []int{1023, 1024} {
				t.Run(fmt.Sprintf("buffer=%d/ending=%q/bytes=%d", bufferSize, ending, size), func(t *testing.T) {
					base := `{"v":3,"type":"hello","firmware":"x"}`
					// CR, if present, is part of the 1023-byte payload budget;
					// only LF is excluded by the protocol.
					payload := base + strings.Repeat(" ", size-len(base)-(len(ending)-1))
					next := `{"v":3,"type":"encoder","seq":2,"delta":1}`
					r := bufio.NewReaderSize(strings.NewReader(payload+ending+next+"\n"), bufferSize)
					frame, err := readFrame(r)
					if size == 1023 {
						if err != nil || string(frame) != payload {
							t.Fatalf("1023-byte frame: bytes=%d error=%v", len(frame), err)
						}
						if _, err := decodeFrame(frame); err != nil {
							t.Fatalf("decode accepted-size frame: %v", err)
						}
					} else if err != errFrame {
						t.Fatalf("1024-byte frame: error=%v, want errFrame", err)
					}
					frame, err = readFrame(r)
					if err != nil || string(frame) != next {
						t.Fatalf("next-line recovery: %q %v", frame, err)
					}
					if _, err := decodeFrame(frame); err != nil {
						t.Fatalf("decode next line: %v", err)
					}
				})
			}
		}
	}
}

func TestFramingRecovery(t *testing.T) {
	r := bufio.NewReaderSize(strings.NewReader(strings.Repeat("x", 4096)+"\n{\"v\":3}\n"), 128)
	if _, err := readFrame(r); err != errFrame {
		t.Fatalf("oversize: %v", err)
	}
	b, err := readFrame(r)
	if err != nil || string(b) != `{"v":3}` {
		t.Fatalf("recovery %q %v", b, err)
	}
}
