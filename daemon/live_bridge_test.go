package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Opt-in integration harness for scripts/check_daemon_modes.py only. Linux PTYs
// lack modem-bit ioctls required by serial.Open, so only that hardware transport
// opening is bypassed; the real session codec/controller/Hyprland backend run.
func TestLivePTYBridge(t *testing.T) {
	path := os.Getenv("EMBEDDECK_TEST_PTY")
	if path == "" {
		t.Skip("explicit live integration opt-in required")
	}
	if !strings.HasPrefix(filepath.Clean(path), "/dev/pts/") {
		t.Fatal("test harness accepts only /dev/pts")
	}
	device, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	c := &Controller{Backend: Hyprland{}, Max: 100}
	err = serveSession(ctx, device, c, nil, time.Second)
	if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
		t.Logf("live harness transport ended: %v", err)
	}
}
