# EmbedDeck USB protocol v1

Transport: USB CDC (development firmware uses ESP32-S3 USB Serial/JTAG), 115200 baud, UTF-8 NDJSON. Each JSON object ends in LF; CRLF is accepted. Maximum line payload: 1023 bytes, not including LF. Oversized or NUL-containing lines are discarded through the next LF. No plain-text application logging shares this stream; ROM boot text may precede the first `hello` and must be ignored.

The physical USB CDC transport is not a keyboard/HID interface. Host-side actions are performed by a daemon. This release implements only a Hyprland backend; Windows/macOS/Android and other desktop backends are not implemented.

## Device → host

Every 2 seconds and at boot:

```json
{"v":1,"type":"hello","firmware":"0.2.0"}
```

Encoder:

```json
{"v":1,"type":"encoder","seq":1,"delta":1}
```

Positive delta requests the next numeric workspace; negative requests the previous. Each event has an integer delta with 1 ≤ abs(delta) ≤ 32. Larger physical movements are split into chunks. No cyclic wrap: host clamps to [1, max-workspace], default 10.

Buttons:

```json
{"v":1,"type":"button","seq":2,"button":"knob","action":"press"}
{"v":1,"type":"button","seq":3,"button":"knob","action":"release"}
```

`button`: `knob` or `user`. `action`: `press` or `release`. Knob press toggles to the previous visited workspace; user press requests workspace 1. Releases perform no action. Sequence numbers identify action acknowledgments; transport is best-effort with no replay/retransmit. Never queue offline physical actions for later playback.

Applied-state readback (on relevant state changes, not every heartbeat):

```json
{"v":1,"type":"status","workspace":2,"connected":true,"rx":1}
```

`rx` counts valid state frames accepted in the current device boot. Status is telemetry, not an action request.

## Host → device

At least once per second and promptly on compositor changes:

```json
{"v":1,"type":"state","workspace":2,"name":"2","workspaces":[1,2,4],"backend":"hyprland","connected":true,"error":""}
```

`workspaces` lists occupied/existing positive workspace IDs, maximum 32. The display shows a 10-workspace page containing the active workspace, underlines existing workspaces and highlights the active one. `name` has a 63-byte firmware buffer; the current UI displays the numeric ID rather than arbitrary workspace names. The bundled font supports ASCII only.

No reachable compositor:

```json
{"v":1,"type":"state","workspace":0,"name":"","workspaces":[],"backend":"hyprland","connected":false,"error":"No Hyprland session"}
```

Action acknowledgment:

```json
{"v":1,"type":"ack","seq":1,"ok":true,"error":""}
```

A negative acknowledgment displays a generic action failure. Host logs contain details. The device displays OFFLINE when state is absent/stale for 3500 ms or `connected` is false, and suppresses action emission until synchronized again. The hello heartbeat continues while offline.

## Security boundary

The device sends enum actions and integer deltas, never arbitrary IPC/shell commands. The host must validate protocol version, types and bounds, construct a fixed dispatcher with a numeric workspace target, use no shell interpolation and keep compositor requests short-lived with deadlines. Do not expose serial-to-IPC forwarding as a network service. State is always refreshed from compositor readback, not merely assumed from a requested target.

## Real-device tests

- `scripts/usb_roundtrip.py`: read actual Hyprland state; wait for firmware hello; send an oversized frame, an incompatible-version frame, then actual state; require a firmware `status` confirming workspace and a positive RX count. Run only with daemon stopped.
- Physical test with daemon: rotate both directions, press knob/user, change workspace from keyboard; compare live compositor state, host logs and the LCD. Do not claim the physical path from fake-serial tests alone.
