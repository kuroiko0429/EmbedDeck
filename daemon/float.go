package main

import (
	"context"
	"errors"
)

func validFloatAction(action string) bool {
	switch action {
	case "move_x", "move_y", "resize_w", "resize_h":
		return true
	}
	return false
}

// Optional so workspace/layout-only backends do not need floating support.
// Float must confirm the pinned target before returning nil. A later Snapshot
// can describe a different focused window and is for publication, not proof.
type floatBackend interface {
	Float(context.Context, string, int) error
}

func (c *Controller) handleFloat(ctx context.Context, f Frame, s State) error {
	if !s.Connected || !s.WindowReady {
		return errors.New("editable window unavailable")
	}
	b, ok := c.Backend.(floatBackend)
	if !ok {
		return errors.New("floating backend unavailable")
	}
	action, delta := c.floatAction, 0
	if f.Type == "button" && f.Button == "knob" && f.Action == "hold" {
		action = "toggle"
	} else if f.Type == "encoder" && f.Delta != nil && *f.Delta != 0 && *f.Delta >= -32 && *f.Delta <= 32 {
		if !s.WindowFloating {
			return errors.New("focused floating window required")
		}
		delta = *f.Delta
	} else {
		return errFrame
	}
	if err := b.Float(ctx, action, delta); err != nil {
		c.Refresh(ctx)
		return errors.New("floating dispatch failed")
	}
	c.Refresh(ctx)
	return nil
}
