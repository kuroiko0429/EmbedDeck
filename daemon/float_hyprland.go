package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strings"
)

var windowAddress = regexp.MustCompile(`^0x[0-9a-fA-F]{1,16}$`)

type editable struct {
	Address    string `json:"address"`
	Mapped     *bool  `json:"mapped"`
	Hidden     *bool  `json:"hidden"`
	Floating   *bool  `json:"floating"`
	Fullscreen *int   `json:"fullscreen"`
	At         []int  `json:"at"`
	Size       []int  `json:"size"`
	Monitor    *int   `json:"monitor"`
	Workspace  struct {
		ID int `json:"id"`
	} `json:"workspace"`
}
type usableArea struct{ left, top, right, bottom int }
type floatMonitorInfo struct {
	ID              *int      `json:"id"`
	X               *int      `json:"x"`
	Y               *int      `json:"y"`
	Width           *int      `json:"width"`
	Height          *int      `json:"height"`
	Scale           *float64  `json:"scale"`
	Transform       *int      `json:"transform"`
	Reserved        []float64 `json:"reserved"`
	Focused         *bool     `json:"focused"`
	ActiveWorkspace struct {
		ID int `json:"id"`
	} `json:"activeWorkspace"`
}

func validCoord(n int) bool     { return n >= -100000 && n <= 100000 }
func validDimension(n int) bool { return n > 0 && n <= 100000 }
func finite(n float64) bool     { return !math.IsNaN(n) && !math.IsInf(n, 0) }
func decodeEditable(wb, mb []byte, workspace int) (editable, usableArea, error) {
	var w editable
	var area usableArea
	invalid := errors.New("editable window unavailable")
	if json.Unmarshal(wb, &w) != nil || !windowAddress.MatchString(w.Address) || w.Mapped == nil || !*w.Mapped || w.Hidden == nil || *w.Hidden || w.Floating == nil || w.Fullscreen == nil || *w.Fullscreen != 0 || w.Monitor == nil || w.Workspace.ID != workspace || workspace < 1 || len(w.At) != 2 || len(w.Size) != 2 {
		return w, area, invalid
	}
	if !validCoord(w.At[0]) || !validCoord(w.At[1]) || !validDimension(w.Size[0]) || !validDimension(w.Size[1]) {
		return w, area, invalid
	}
	var monitors []floatMonitorInfo
	if json.Unmarshal(mb, &monitors) != nil {
		return w, area, invalid
	}
	found := false
	for _, m := range monitors {
		if m.ID == nil || *m.ID != *w.Monitor {
			continue
		}
		if found {
			return w, area, invalid
		}
		found = true
		if m.X == nil || m.Y == nil || !validCoord(*m.X) || !validCoord(*m.Y) || m.Width == nil || m.Height == nil || !validDimension(*m.Width) || !validDimension(*m.Height) || m.Scale == nil || !finite(*m.Scale) || *m.Scale <= 0 || m.Transform == nil || *m.Transform < 0 || *m.Transform > 7 || len(m.Reserved) != 4 || m.Focused == nil || !*m.Focused || m.ActiveWorkspace.ID != workspace {
			return w, area, invalid
		}
		for _, v := range m.Reserved {
			if !finite(v) || v < 0 || v > 100000 {
				return w, area, invalid
			}
		}
		width, height := *m.Width, *m.Height
		if *m.Transform%2 != 0 {
			width, height = height, width
		}
		left := float64(*m.X) + m.Reserved[0]
		top := float64(*m.Y) + m.Reserved[1]
		right := float64(*m.X) + float64(width) / *m.Scale - m.Reserved[2]
		bottom := float64(*m.Y) + float64(height) / *m.Scale - m.Reserved[3]
		// Round inward: no generated logical pixel can cross a fractional edge.
		if !finite(left) || !finite(top) || !finite(right) || !finite(bottom) || left < -100000 || top < -100000 || right > 100000 || bottom > 100000 || right-left < 1 || bottom-top < 1 || right-left > 100000 || bottom-top > 100000 {
			return w, area, invalid
		}
		area = usableArea{int(math.Ceil(left)), int(math.Ceil(top)), int(math.Floor(right)), int(math.Floor(bottom))}
		if area.right <= area.left || area.bottom <= area.top {
			return w, area, invalid
		}
	}
	if !found {
		return w, area, invalid
	}
	return w, area, nil
}
func applyEditable(s *State, w editable) {
	s.WindowReady = true
	s.WindowFloating = *w.Floating
	s.WindowX = w.At[0]
	s.WindowY = w.At[1]
	s.WindowWidth = w.Size[0]
	s.WindowHeight = w.Size[1]
}
func readEditable(ctx context.Context, p string) (editable, usableArea, int, error) {
	var a struct {
		ID int `json:"id"`
	}
	b, err := request(ctx, p, "j/activeworkspace")
	if err != nil {
		return editable{}, usableArea{}, 0, err
	}
	if json.Unmarshal(b, &a) != nil || a.ID < 1 || a.ID > 10000 {
		return editable{}, usableArea{}, 0, errors.New("invalid active workspace")
	}
	wb, err := request(ctx, p, "j/activewindow")
	if err != nil {
		return editable{}, usableArea{}, 0, err
	}
	mb, err := request(ctx, p, "j/monitors")
	if err != nil {
		return editable{}, usableArea{}, 0, err
	}
	w, area, err := decodeEditable(wb, mb, a.ID)
	return w, area, a.ID, err
}
func clampInt(n, lo, hi int) int { return min(max(n, lo), hi) }

// Float only constructs native Lua commands from validated compositor geometry
// and an allowlisted action. Every operation pins the original window address.
func (h Hyprland) Float(ctx context.Context, action string, delta int) error {
	if (action == "toggle" && delta != 0) || (action != "toggle" && (!validFloatAction(action) || delta == 0 || delta < -32 || delta > 32)) {
		return errors.New("invalid floating action")
	}
	p, err := h.path(ctx)
	if err != nil {
		return err
	}
	w, area, workspace, err := readEditable(ctx, p)
	if err != nil {
		return err
	}
	desired := *w.Floating
	command := ""
	if action == "toggle" {
		desired = !*w.Floating
		verb := "disable"
		if desired {
			verb = "enable"
		}
		command = fmt.Sprintf(`dispatch hl.dsp.window.float({ action = "%s", window = "address:%s" })`, verb, w.Address)
	} else {
		if !*w.Floating {
			return errors.New("focused floating window required")
		}
		x, y, width, height := w.At[0], w.At[1], w.Size[0], w.Size[1]
		areaWidth, areaHeight := area.right-area.left, area.bottom-area.top
		if action == "move_x" || action == "move_y" {
			// An oversized window cannot be moved wholly into the workarea; resizing
			// it implicitly would violate the selected action. Fail closed instead.
			if width > areaWidth || height > areaHeight {
				return errors.New("window exceeds usable area")
			}
			if action == "move_x" {
				x += delta * 10
			} else {
				y += delta * 10
			}
			x = clampInt(x, area.left, area.right-width)
			y = clampInt(y, area.top, area.bottom-height)
			command = fmt.Sprintf(`dispatch hl.dsp.window.move({ x = %d, y = %d, relative = false, window = "address:%s" })`, x, y, w.Address)
		} else {
			if x < area.left || y < area.top || x >= area.right || y >= area.bottom {
				return errors.New("window origin outside usable area")
			}
			maxWidth, maxHeight := area.right-x, area.bottom-y
			if action == "resize_w" {
				width += delta * 10
			} else {
				height += delta * 10
			}
			minWidth, minHeight := min(160, areaWidth), min(100, areaHeight)
			if maxWidth < minWidth || maxHeight < minHeight {
				return errors.New("window origin cannot fit minimum size")
			}
			width = clampInt(width, minWidth, maxWidth)
			height = clampInt(height, minHeight, maxHeight)
			command = fmt.Sprintf(`dispatch hl.dsp.window.resize({ x = %d, y = %d, relative = false, window = "address:%s" })`, width, height, w.Address)
		}
	}
	b, err := request(ctx, p, command)
	if err != nil {
		return err
	}
	if strings.TrimSpace(string(b)) != "ok" {
		return errors.New("dispatch rejected")
	}
	// Read the pinned client rather than a possibly different focused window.
	after, actualArea, err := readPinnedEditable(ctx, p, w.Address, workspace)
	if err != nil {
		return err
	}
	if *after.Floating != desired {
		return errors.New("floating state not confirmed")
	}
	if action == "toggle" {
		return nil
	}
	if after.Size[0] > actualArea.right-actualArea.left || after.Size[1] > actualArea.bottom-actualArea.top {
		return errors.New("actual window exceeds usable area")
	}
	if action == "resize_w" || action == "resize_h" {
		// Native floating resize can preserve the center, shifting the origin.
		// Keep the previous top-left anchor where possible, using actual sizes
		// (including application constraints), and clamp it to the workarea.
		x := clampInt(w.At[0], actualArea.left, actualArea.right-after.Size[0])
		y := clampInt(w.At[1], actualArea.top, actualArea.bottom-after.Size[1])
		if after.At[0] != x || after.At[1] != y {
			command = fmt.Sprintf(`dispatch hl.dsp.window.move({ x = %d, y = %d, relative = false, window = "address:%s" })`, x, y, w.Address)
			b, err = request(ctx, p, command)
			if err != nil {
				return err
			}
			if strings.TrimSpace(string(b)) != "ok" {
				return errors.New("resize anchor dispatch rejected")
			}
			after, actualArea, err = readPinnedEditable(ctx, p, w.Address, workspace)
			if err != nil {
				return err
			}
			if !*after.Floating || after.At[0] != x || after.At[1] != y {
				return errors.New("resize anchor not confirmed")
			}
		}
	}
	if after.At[0] < actualArea.left || after.At[1] < actualArea.top ||
		after.At[0]+after.Size[0] > actualArea.right || after.At[1]+after.Size[1] > actualArea.bottom {
		return errors.New("actual window outside usable area")
	}
	return nil
}

func readPinnedEditable(ctx context.Context, p, address string, workspace int) (editable, usableArea, error) {
	clients, err := request(ctx, p, "j/clients")
	if err != nil {
		return editable{}, usableArea{}, err
	}
	var list []json.RawMessage
	if json.Unmarshal(clients, &list) != nil {
		return editable{}, usableArea{}, errors.New("invalid client readback")
	}
	mb, err := request(ctx, p, "j/monitors")
	if err != nil {
		return editable{}, usableArea{}, err
	}
	for _, raw := range list {
		var target struct {
			Address string `json:"address"`
		}
		if json.Unmarshal(raw, &target) != nil {
			return editable{}, usableArea{}, errors.New("invalid client readback")
		}
		if target.Address != address {
			continue
		}
		return decodeEditable(raw, mb, workspace)
	}
	return editable{}, usableArea{}, errors.New("target window disappeared")
}
