# EmbedDeck USB protocol v3 / firmware 0.4.0

USB CDC, 115200 baud, UTF-8 NDJSON. One JSON object per LF (CRLF accepted), maximum 1023 payload bytes excluding LF. Discard oversized/NUL-containing frames through LF. Ignore ESP-ROM boot text before hello. No plain-text firmware application logging is mixed into this stream.

v3 is deliberately incompatible with v1/v2: both firmware and daemon must be updated together. The extra FLOAT mode and required geometry fields must not be silently interpreted by an older peer. Old frames are rejected. USB is not HID; host actions require the Go daemon. Current backend: Hyprland 0.57 Lua plus hyprscroller-ng's `hyprscrolling` layout; FLOAT uses native window dispatchers and does not require that plugin.

## Device -> host

At boot and every 2 seconds:

```json
{"v":3,"type":"hello","firmware":"0.4.0"}
```

Encoder event (`1 <= abs(delta) <= 32`, split larger physical deltas):

```json
{"v":3,"type":"encoder","seq":1,"delta":1}
```

Debounced gesture:

```json
{"v":3,"type":"button","seq":2,"button":"user","action":"click"}
{"v":3,"type":"button","seq":3,"button":"knob","action":"hold"}
```

`button` is `knob` or `user`; `action` is `click` or `hold`. Click fires on release before 600ms; hold fires once at/after 600ms and consumes release, so it never also clicks. Raw press/release events are not emitted. This is best-effort serial transport: no replay/retransmit; sequence numbers correlate ACKs, not a durable delivery queue.

Applied host-state readback on relevant changes:

```json
{"v":3,"type":"status","workspace":2,"connected":true,"rx":1,"mode":"float","focus_axis":"horizontal","width_mode":"fine","float_action":"move_x","window_floating":true,"window_ready":true,"window_x":100,"window_y":100,"window_width":600,"window_height":400}
```

`rx` counts valid accepted host states in this boot. Status is telemetry, never an action.

## Host -> device

On changes and at least every second:

```json
{"v":3,"type":"state","workspace":2,"name":"2","workspaces":[1,2],"backend":"hyprland","connected":true,"error":"","mode":"float","focus_axis":"horizontal","width_mode":"fine","layout":"hyprscrolling","app":"kitty","window_width":600,"width_percent":32,"float_action":"move_x","window_floating":true,"window_ready":true,"window_x":100,"window_y":100,"window_height":400}
```

Required enums:

- mode: `workspace`, `focus`, `width`, `float`
- focus_axis: `horizontal`, `vertical`
- width_mode: `fine`, `preset`
- float_action: `move_x`, `move_y`, `resize_w`, `resize_h`

All fields in the example are required by firmware except `error` (empty default). The active workspace is in [1,10000] (0 only when disconnected); larger IDs are unsupported and rejected before rendering. `workspaces` has <=32 integer IDs. `layout` comes from the active workspace's tiledLayout. `app` is an ASCII app-class label (24-byte display buffer); `name` has a 63-byte buffer. window_width is current focused-window logical pixel width (0 when none). width_percent is an approximate percentage of monitor usable logical width, clamped to [0,100], not an exact plugin column-width query: gaps/borders and fullscreen-single-column can change apparent size. The UI marks it with `~`.

If the backend is unavailable, send connected=false, workspace=0, name="", workspaces=[], empty app/layout and zero widths, but preserve the controller's selected mode/axis/width_mode.

Required geometry fields: `window_floating` and `window_ready` are booleans; `window_x`/`window_y` are signed logical desktop coordinates in [-100000,100000]; `window_height` is in [0,100000]. Width retains its existing bounds. `window_ready` means an eligible mapped, visible, nonfullscreen focused window on the current workspace with valid geometry and a usable monitor work area. It can be true for a tiled window (eligible for toggling floating). If true, both dimensions must be positive. A negative position is valid on monitors to the left/above the origin. An absent/unavailable target uses ready=false. Preserve `float_action` on backend loss. Status echoes these fields plus `window_width` so actual device application can be verified.

```json
{"v":3,"type":"ack","seq":1,"ok":true,"error":""}
```

ACK false shows a generic firmware warning; daemon logs contain action results. Desktop operations must query compositor state afterward rather than predict geometry. Some operations intentionally have no visible effect (one-window focus, width clamp, single-column full-width); successful dispatcher acceptance is not proof of a changed window.

## Mode/button contract

The host owns mode, horizontal/vertical axis and fine/preset selection. Defaults: workspace/horizontal/fine. They survive serial reconnect within the same daemon process, but not daemon restart.

| Input | workspace | focus | width | float |
| --- | --- | --- | --- | --- |
| Turn + / - | Next / previous numeric workspace | Right / left or down / up | + / - 5 percentage points or preset | Adjust selected geometry component by delta * 10 logical pixels |
| Knob click | Previous visited workspace | Toggle horizontal / vertical | Toggle fine / preset | move_x -> move_y -> resize_w -> resize_h -> move_x |
| Knob hold | Workspace1 | Focus history back | Fit active column to screen | Toggle floating / tiled |
| User click | Next mode | Next mode | Next mode | Next mode |
| User hold | Workspace mode | Workspace mode | Workspace mode | Workspace mode |

Mode cycle: workspace -> focus -> width -> float -> workspace. Numeric workspace navigation clamps to [1,max-workspace], default10. User hold changes mode only, never moves the desktop. Mode/axis/float-action selection can operate while the daemon link is alive but the compositor is unavailable. Float action defaults to move_x and is host-owned like the other mode settings.

Plugin actions require active layout `hyprscrolling` and a mapped, nonfloating, nonfullscreen focused window. Workspace actions remain usable without the plugin. Messages are fixed templates: `focus l/r/u/d`, `focusback`, `colresize +/-0.05` (scaled by bounded delta), `colresize +/-conf`, `fit active`; never pass serial-provided Lua/shell strings to IPC.

FLOAT toggle requires an eligible window; geometry edits additionally require floating=true. Fullscreen, hidden, unmapped, absent or invalid targets fail closed. The daemon constructs native `hl.dsp.window.float`, `move` and `resize` calls using validated numeric values and a validated compositor-originated address selector, never a serial-provided Lua string. Edits are clamped to the monitor's usable logical rectangle (position, scale, transform and reserved areas), with a requested minimum 160x100 pixels or the smaller available work area. Application size constraints may limit changes. Query actual geometry afterward; do not display requested coordinates as confirmed. Returning to workspace mode does not automatically re-tile a window.

Native resize may recenter a floating window. After reading the actual dimensions, preserve the old top-left anchor where possible, clamped to the current workarea, with at most one address-pinned corrective move and a second readback. Reject an oversized application-constrained result or an unconfirmed correction rather than acknowledge an offscreen result. Already oversized windows cannot be moved wholly inside without resizing and fail closed; resize also refuses origins that cannot fit the requested minimum instead of silently relocating the target first.

The backend confirms its pinned operation target before returning success. A subsequent snapshot publishes the currently focused window, which may be different; do not compare that window's floating flag with an earlier target's flag as a second operation confirmation. An ACK describes the verified operation; the state frame describes current focus.

Firmware marks the host stale after 3500ms without a valid state. It sends no actions while stale and never replays offline input. With a live host but unavailable compositor, mode gestures may still be sent; rotation is suppressed.

## Tests

- `bash scripts/test_firmware.sh`: debounce, gesture timing/rollover, v3 geometry schema, maximum state, frame recovery, four portrait tabs, FLOAT text and LED colors.
- `go test -race ./...` and `go vet ./...` in daemon/: protocol, mode machine, backend guards and IPC.
- `scripts/usb_roundtrip.py` (daemon stopped): actual Hyprland snapshot -> USB -> actual firmware state readback, after oversized/incompatible test inputs.
- `scripts/check_hyprscrolling.py`: real plugin actions on two owned temporary kitty windows in an unused workspace, then close them and restore prior workspace/focus. No existing windows are moved/resized.
- `scripts/check_floating.py`: native floating toggle, X/Y movement and width/height changes on owned windows only, with actual geometry readback and cleanup.
- Physical tests must separately verify real encoder/gestures -> Go -> IPC and LCD feedback. Fake serial tests alone do not prove that path.
