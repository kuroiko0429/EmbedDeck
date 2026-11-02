package main

import (
	"context"
	"fmt"
	"testing"
)

func TestDesktopRejectsUnsupportedWorkspaceRange(t *testing.T) {
	for _, id := range []int{10001, 2147483647} {
		t.Run(fmt.Sprint(id), func(t *testing.T) {
			h := fakeDesktop(t, func(command string) string {
				if command == "j/activeworkspace" {
					return fmt.Sprintf(`{"id":%d,"name":"large","tiledLayout":"hyprscrolling"}`, id)
				}
				return `[]`
			})
			if state, err := h.Snapshot(context.Background()); err == nil || state.Connected {
				t.Fatalf("unsupported workspace must fail closed: state=%+v error=%v", state, err)
			}
		})
	}
}
