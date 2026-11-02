# EmbedDeck USB protocol v2 / firmware 0.3.0

USB CDC, 115200 baud, UTF-8 NDJSON. One JSON object per LF (CRLF accepted), maximum 1023 payload bytes excluding LF. Discard oversized/NUL-containing frames through LF. Ignore ESP-ROM boot text before hello. No plain-text firmware application logging is mixed into this stream.

v2 is deliberately incompatible with v1: both firmware and daemon must be updated together. Old frames are rejected instead of silently changing workspace in a new mode. USB is not HID; host actions require the Go daemon. Current backend: Hyprland 0.57 Lua plus hyprscroller-ng's `hyprscrolling` layout.

## Device -> host

At boot and every 2 seconds:

```json
{"v":2,"type":"hello","firmware":"0.3.0"}
```

Encoder event (`1 <= abs(delta) <= 32`, split larger physical deltas):

```json
{"v":2,"type":"encoder","seq":1,"delta":1}
```

Debounced gesture:

```json
{"v":2,"type":"button","seq":2,"button":"user","action":"click"}
{"v":2,"type":"button","seq":3,"button":"knob","action":"hold"}
```

`button` is `knob` or `user`; `action` is `click` or `hold`. Click fires on release before 600ms; hold fires once at/after 600ms and consumes release, so it never also clicks. Raw press/release events are not emitted in v2. This is best-effort serial transport: no replay/retransmit; sequence numbers correlate ACKs, not a durable delivery queue.

Applied host-state readback on relevant changes:

```json
{"v":2,"type":"status","workspace":2,"connected":true,"rx":1,"mode":"width","focus_axis":"horizontal","width_mode":"fine"}
```

`rx` counts valid accepted host states in this boot. Status is telemetry, never an action.

## Host -> device

On changes and at least every second:

```json
{"v":2,"type":"state","workspace":2,"name":"2","workspaces":[1,2],"backend":"hyprland","connected":true,"error":"","mode":"width","focus_axis":"horizontal","width_mode":"fine","layout":"hyprscrolling","app":"kitty","window_width":933,"width_percent":49}
```

Required enums:

- mode: `workspace`, `focus`, `width`
- focus_axis: `horizontal`, `vertical`
- width_mode: `fine`, `preset`

All fields in the example are required by firmware except `error` (empty default). The active workspace is in [1,10000] (0 only when disconnected); larger IDs are unsupported and rejected before rendering. `workspaces` has <=32 integer IDs. `layout` comes from the active workspace's tiledLayout. `app` is an ASCII app-class label (24-byte display buffer); `name` has a 63-byte buffer. window_width is current focused-window logical pixel width (0 when none). width_percent is an approximate percentage of monitor usable logical width, clamped to [0,100], not an exact plugin column-width query: gaps/borders and fullscreen-single-column can change apparent size. The UI marks it with `~`.

If the backend is unavailable, send connected=false, workspace=0, name="", workspaces=[], empty app/layout and zero widths, but preserve the controller's selected mode/axis/width_mode.

```json
{"v":2,"type":"ack","seq":1,"ok":true,"error":""}
```

ACK false shows a generic firmware warning; daemon logs contain action results. Desktop operations must query compositor state afterward rather than predict geometry. Some operations intentionally have no visible effect (one-window focus, width clamp, single-column full-width); successful dispatcher acceptance is not proof of a changed window.

## Mode/button contract

The host owns mode, horizontal/vertical axis and fine/preset selection. Defaults: workspace/horizontal/fine. They survive serial reconnect within the same daemon process, but not daemon restart.

| Input | workspace | focus | width |
| --- | --- | --- | --- |
| Turn + / - | Next / previous numeric workspace | Right / left or down / up | + / - 5 percentage points or preset |
| Knob click | Previous visited workspace | Toggle horizontal / vertical | Toggle fine / preset |
| Knob hold | Workspace1 | Focus history back | Fit active column to screen |
| User click | Next mode | Next mode | Next mode |
| User hold | Workspace mode | Workspace mode | Workspace mode |

Mode cycle: workspace -> focus -> width -> workspace. Numeric workspace navigation clamps to [1,max-workspace], default10. User hold changes mode only, never moves the desktop. Mode/axis selection can operate while the daemon link is alive but the compositor is unavailable.

Plugin actions require active layout `hyprscrolling` and a mapped, nonfloating, nonfullscreen focused window. Workspace actions remain usable without the plugin. Messages are fixed templates: `focus l/r/u/d`, `focusback`, `colresize +/-0.05` (scaled by bounded delta), `colresize +/-conf`, `fit active`; never pass serial-provided Lua/shell strings to IPC.

Firmware marks the host stale after 3500ms without a valid state. It sends no actions while stale and never replays offline input. With a live host but unavailable compositor, mode gestures may still be sent; rotation is suppressed.

## Tests

- `bash scripts/test_firmware.sh`: debounce, gesture timing/rollover, v2 state schema, frame recovery, portrait layout.
- `go test -race ./...` and `go vet ./...` in daemon/: protocol, mode machine, backend guards and IPC.
- `scripts/usb_roundtrip.py` (daemon stopped): actual Hyprland snapshot -> USB -> actual firmware state readback, after oversized/incompatible test inputs.
- `scripts/check_hyprscrolling.py`: real plugin actions on two owned temporary kitty windows in an unused workspace, then close them and restore prior workspace/focus. No existing windows are moved/resized.
- Physical tests must separately verify real encoder/gestures -> Go -> IPC and LCD feedback. Fake serial tests alone do not prove that path.
