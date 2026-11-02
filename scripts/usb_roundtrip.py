#!/usr/bin/env python3
"""Real-device smoke test; stop the daemon before using this serial port."""
import json
from pathlib import Path
import serial
import subprocess
import time

state = json.loads(subprocess.check_output(['./daemon/embeddeck', '--probe'], text=True))
assert state['v'] == 3 and state['connected'], 'Build the v3 daemon and check Hyprland first'
port = '/dev/serial/by-id/usb-Espressif_USB_JTAG_serial_debug_unit_30:ED:A0:34:56:4C-if00'
frames = []
with serial.Serial(port=None, baudrate=115200, timeout=.2) as device:
    device.dtr = False
    device.rts = False
    device.port = port
    device.open()
    # Wait for a real firmware hello before injecting test input (not ROM boot).
    handshake_deadline = time.monotonic() + 6
    while time.monotonic() < handshake_deadline:
        line = device.readline()
        try:
            frame = json.loads(line)
        except (ValueError, UnicodeError):
            continue
        if frame.get('v') == 3 and frame.get('type') == 'hello' and frame.get('firmware') == '0.4.0':
            print('Firmware hello:', frame)
            break
    else:
        raise SystemExit('FAIL: firmware hello not received')
    # Explicit adversarial input, not fabricated output: recovery must work on the board.
    device.write(b'x' * 2048 + b'\n{"v":999,"type":"state"}\n')
    deadline = time.monotonic() + 7
    last_send = 0
    while time.monotonic() < deadline:
        if time.monotonic() - last_send > .7:
            device.write(json.dumps(state, separators=(',', ':')).encode() + b'\n')
            last_send = time.monotonic()
        line = device.readline()
        if not line:
            continue
        try:
            frame = json.loads(line)
        except (ValueError, UnicodeError):
            print('Non-JSON boot bytes:', repr(line))
            continue
        frames.append(frame)
        print(json.dumps(frame), flush=True)
        if (frame.get('v') == 3 and frame.get('type') == 'status' and frame.get('workspace') == state['workspace']
                and frame.get('connected') is True and frame.get('rx', 0) > 0
                and frame.get('mode') == state['mode'] and frame.get('focus_axis') == state['focus_axis']
                and frame.get('width_mode') == state['width_mode']
                and all(frame.get(k) == state[k] for k in ('float_action', 'window_ready', 'window_floating',
                                                           'window_x', 'window_y', 'window_width', 'window_height'))):
            break
    else:
        raise SystemExit('FAIL: no actual firmware state readback over USB')
Path('backups/usb-roundtrip.json').write_text(json.dumps(dict(host_state=state, device_frames=frames), indent=2))
print('PASS: actual Hyprland snapshot -> USB JSON -> firmware status readback; oversized-frame recovery')
