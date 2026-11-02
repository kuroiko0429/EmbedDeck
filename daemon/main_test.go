package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
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
				fmt.Fprintf(c, `{"id":2,"name":%q,"tiledLayout":%q}`, strings.Repeat("<", 63), strings.Repeat("<", 63))
			case "j/workspaces":
				var list []map[string]int
				for i := 0; i < 31; i++ {
					list = append(list, map[string]int{"id": 2147483647 - i})
				}
				json.NewEncoder(c).Encode(list)
			case "j/activewindow":
				fmt.Fprintf(c, `{"address":"0xabc","mapped":true,"floating":false,"fullscreen":0,"class":%q,"size":[1000,700],"monitor":1,"workspace":{"id":2}}`, strings.Repeat("<", 24))
			case "j/monitors":
				io.WriteString(c, `[{"id":1,"width":1920,"height":1080,"scale":1}]`)
			}
			c.Close()
		}
	}()
	var out bytes.Buffer
	if e := run(context.Background(), []string{"--probe", "--port", "/does/not/exist"}, &out); e != nil {
		t.Fatal(e)
	}
	if out.Len()-1 > maxFrame {
		t.Fatalf("probe frame too large: %d", out.Len()-1)
	}
	var s State
	if e = json.Unmarshal(out.Bytes(), &s); e != nil || !s.Connected || s.Workspace != 2 || s.V != 3 || s.Backend != "hyprland" {
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
