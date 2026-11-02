package main

import (
	"fmt"
	"strings"
	"testing"
)

func TestEncoderDeltaBounds(t *testing.T) {
	for _, delta := range []int{-33, -32, -1, 0, 1, 32, 33} {
		t.Run(fmt.Sprint(delta), func(t *testing.T) {
			frame := fmt.Sprintf(`{"v":1,"type":"encoder","seq":1,"delta":%d}`, delta)
			_, err := decodeFrame([]byte(frame))
			valid := delta >= -32 && delta <= 32 && delta != 0
			if (err == nil) != valid {
				t.Fatalf("delta=%d: error=%v, want valid=%v", delta, err, valid)
			}
		})
	}
}

func TestDecoderFrameByteLimit(t *testing.T) {
	for _, size := range []int{1023, 1024} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			base := `{"v":1,"type":"hello","firmware":"x"}`
			frame := base + strings.Repeat(" ", size-len(base))
			_, err := decodeFrame([]byte(frame))
			if size == 1023 && err != nil {
				t.Fatalf("rejected 1023-byte JSON: %v", err)
			}
			if size == 1024 && err != errFrame {
				t.Fatalf("1024-byte JSON: error=%v, want errFrame", err)
			}
		})
	}
}

func TestProtocolValidation(t *testing.T) {
	for _, s := range []string{`{"v":1,"type":"hello","firmware":"0.2.0"}`, `{"v":1,"type":"encoder","seq":0,"delta":-32}`, `{"v":1,"type":"button","seq":4294967295,"button":"knob","action":"press"}`} {
		if _, err := decodeFrame([]byte(s)); err != nil {
			t.Fatalf("valid %s: %v", s, err)
		}
	}
	for _, s := range []string{`{`, `{}`, `{"v":2,"type":"hello","firmware":"x"}`, `{"v":1,"type":"exec"}`, `{"v":1,"type":"encoder","seq":1,"delta":33}`, `{"v":1,"type":"encoder","seq":-1,"delta":1}`, `{"v":1,"type":"encoder","delta":1}`, `{"v":1,"type":"encoder","seq":1,"delta":1.5}`, `{"v":1,"type":"button","seq":1,"button":"shell","action":"press"}`, `{"v":1,"type":"button","seq":1,"button":"user","action":"run"}`, `{"v":1,"type":"hello","firmware":"x","command":"bad"}`, `{"v":1,"type":"hello","firmware":"x"} {}`, `{"v":1,"v":1,"type":"hello","firmware":"x"}`, strings.Repeat("x", 1025)} {
		if _, err := decodeFrame([]byte(s)); err == nil {
			t.Errorf("accepted %s", s)
		}
	}
}
