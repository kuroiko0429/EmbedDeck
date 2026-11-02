package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"
)

func TestProbeNeverOpensSerial(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", root)
	t.Setenv("HYPRLAND_INSTANCE_SIGNATURE", "probe")
	dir := filepath.Join(root, "hypr", "probe")
	os.MkdirAll(dir, 0700)
	l, e := net.Listen("unix", filepath.Join(dir, ".socket.sock"))
	if e != nil {
		t.Fatal(e)
	}
	defer l.Close()
	go func() {
		for {
			c, e := l.Accept()
			if e != nil {
				return
			}
			b := make([]byte, 4096)
			n, _ := c.Read(b)
			switch string(b[:n]) {
			case "j/activeworkspace":
				io.WriteString(c, `{"id":2,"name":"two"}`)
			case "j/workspaces":
				io.WriteString(c, `[{"id":1},{"id":2}]`)
			}
			c.Close()
		}
	}()
	var out bytes.Buffer
	if e := run(context.Background(), []string{"--probe", "--port", "/does/not/exist"}, &out); e != nil {
		t.Fatal(e)
	}
	var s State
	if e = json.Unmarshal(out.Bytes(), &s); e != nil || !s.Connected || s.Workspace != 2 || s.V != 1 || s.Backend != "hyprland" {
		t.Fatal(out.String(), e)
	}
	for _, args := range [][]string{{"--max-workspace", "0"}, {"--max-workspace", "10001"}, {"extra"}} {
		if e := run(context.Background(), args, &bytes.Buffer{}); e == nil {
			t.Fatal(args)
		}
	}
	mode := serialMode()
	if mode.InitialStatusBits == nil || mode.InitialStatusBits.DTR || mode.InitialStatusBits.RTS {
		t.Fatal(mode)
	}
}
