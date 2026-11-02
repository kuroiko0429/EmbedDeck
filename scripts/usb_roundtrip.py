#!/usr/bin/env python3
"""Real-device smoke test; stop the daemon before using this serial port."""
import json
import os
from pathlib import Path
import serial
import socket
import time

runtime = Path(os.environ.get('XDG_RUNTIME_DIR', f'/run/user/{os.getuid()}'))
sockets = list((runtime / 'hypr').glob('*/.socket.sock'))
assert len(sockets) == 1, 'Select exactly one live Hyprland instance'

def request(command):
    with socket.socket(socket.AF_UNIX) as conn:
        conn.settimeout(2)
        conn.connect(str(sockets[0]))
        conn.sendall(command.encode())
        result = bytearray()
        while data := conn.recv(65536):
            result.extend(data)
    return json.loads(result)

active = request('j/activeworkspace')
state = dict(v=1, type='state', workspace=active['id'], name=active['name'],
             workspaces=[x['id'] for x in request('j/workspaces') if x['id'] > 0],
             backend='hyprland', connected=True, error='')
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
        if frame.get('type') == 'hello' and frame.get('firmware') == '0.2.0':
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
        if (frame.get('type') == 'status' and frame.get('workspace') == active['id']
                and frame.get('connected') is True and frame.get('rx', 0) > 0):
            break
    else:
        raise SystemExit('FAIL: no actual firmware state readback over USB')
Path('backups/usb-roundtrip.json').write_text(json.dumps(dict(host_state=state, device_frames=frames), indent=2))
print('PASS: actual Hyprland snapshot -> USB JSON -> firmware status readback; oversized-frame recovery')
