package main

import (
	"fmt"
	"strings"
	"testing"
)

func TestEncoderDeltaBounds(t *testing.T) {
	for _, delta := range []int{-33, -32, -1, 0, 1, 32, 33} {
		t.Run(fmt.Sprint(delta), func(t *testing.T) {
			frame := fmt.Sprintf(`{"v":3,"type":"encoder","seq":1,"delta":%d}`, delta)
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
			base := `{"v":3,"type":"hello","firmware":"x"}`
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
	for _, s := range []string{`{"v":3,"type":"hello","firmware":"0.4.0"}`, `{"v":3,"type":"encoder","seq":0,"delta":-32}`, `{"v":3,"type":"button","seq":4294967295,"button":"knob","action":"click"}`} {
		if _, err := decodeFrame([]byte(s)); err != nil {
			t.Fatalf("valid %s: %v", s, err)
		}
	}
	for _, s := range []string{`{`, `{}`, `{"v":0,"type":"hello","firmware":"x"}`, `{"v":3,"type":"exec"}`, `{"v":3,"type":"encoder","seq":1,"delta":33}`, `{"v":3,"type":"encoder","seq":-1,"delta":1}`, `{"v":3,"type":"encoder","delta":1}`, `{"v":3,"type":"encoder","seq":1,"delta":1.5}`, `{"v":3,"type":"button","seq":1,"button":"shell","action":"click"}`, `{"v":3,"type":"button","seq":1,"button":"user","action":"run"}`, `{"v":3,"type":"hello","firmware":"x","command":"bad"}`, `{"v":3,"type":"hello","firmware":"x"} {}`, `{"v":3,"v":3,"type":"hello","firmware":"x"}`, strings.Repeat("x", 1025)} {
		if _, err := decodeFrame([]byte(s)); err == nil {
			t.Errorf("accepted %s", s)
		}
	}
}

func TestV3GesturesAndTelemetry(t *testing.T) {
	for _, action := range []string{"click", "hold"} {
		for _, button := range []string{"knob", "user"} {
			if _, err := decodeFrame([]byte(fmt.Sprintf(`{"v":3,"type":"button","seq":1,"button":%q,"action":%q}`, button, action))); err != nil {
				t.Fatal(err)
			}
		}
	}
	for _, line := range []string{
		`{"v":1,"type":"hello","firmware":"0.2.0"}`,
		`{"v":3,"type":"button","seq":1,"button":"user","action":"press"}`,
		`{"v":3,"type":"button","seq":1,"button":"knob","action":"release"}`,
		`{"v":3,"type":"status","workspace":2,"connected":true,"rx":1,"mode":"shell","focus_axis":"horizontal","width_mode":"fine","float_action":"move_x","window_floating":false,"window_ready":false,"window_x":0,"window_y":0,"window_width":0,"window_height":0}`,
		`{"v":3,"type":"status","workspace":2,"connected":true,"rx":1,"mode":"focus","focus_axis":"bad","width_mode":"fine","float_action":"move_x","window_floating":false,"window_ready":false,"window_x":0,"window_y":0,"window_width":0,"window_height":0}`,
		`{"v":3,"type":"status","workspace":2,"connected":true,"rx":1,"mode":"focus","focus_axis":"vertical","width_mode":"bad","float_action":"move_x","window_floating":false,"window_ready":false,"window_x":0,"window_y":0,"window_width":0,"window_height":0}`,
		`{"v":3,"type":"status","workspace":2,"connected":true,"rx":1}`,
	} {
		if _, err := decodeFrame([]byte(line)); err == nil {
			t.Fatalf("accepted %s", line)
		}
	}
}
