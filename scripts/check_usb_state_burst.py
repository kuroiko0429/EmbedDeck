#!/usr/bin/env python3
"""Actual USB throughput regression. Stop embeddeck.service before running."""
import json
from pathlib import Path
import subprocess
import time
import serial

state = json.loads(subprocess.check_output(['./daemon/embeddeck', '--probe'], text=True))
port = '/dev/serial/by-id/usb-Espressif_USB_JTAG_serial_debug_unit_30:ED:A0:34:56:4C-if00'
statuses = []
with serial.Serial(port=None, baudrate=115200, timeout=.05) as device:
    device.dtr = False; device.rts = False; device.port = port; device.open()
    device.reset_input_buffer()
    # Wait for boot/old host traffic to finish before taking the RX baseline.
    end = time.monotonic() + 6
    while time.monotonic() < end:
        try:
            frame = json.loads(device.readline())
        except (ValueError, UnicodeError):
            continue
        if frame.get('v') == 2 and frame.get('type') == 'hello': break
    else:
        raise SystemExit('No v2 firmware hello')
    state['mode'] = 'focus'
    baseline = None
    end = time.monotonic() + 3
    # Send exactly once: retries here would corrupt the counter difference.
    device.write(json.dumps(state, separators=(',', ':')).encode() + b'\n')
    while time.monotonic() < end and baseline is None:
        try:
            frame = json.loads(device.readline())
        except (ValueError, UnicodeError):
            continue
        if frame.get('type') == 'status' and frame.get('mode') == 'focus':
            baseline = frame['rx']
    assert baseline is not None, 'No firmware RX baseline'
    count = 16
    payload = b''
    for n in range(count):
        state['mode'] = 'width' if n % 2 == 0 else 'focus'
        payload = json.dumps(state, separators=(',', ':')).encode() + b'\n'
        device.write(payload)
        end = time.monotonic() + .15
        while time.monotonic() < end:
            try:
                frame = json.loads(device.readline())
            except (ValueError, UnicodeError):
                continue
            if frame.get('type') == 'status': statuses.append(frame)
    end = time.monotonic() + .5
    while time.monotonic() < end:
        try:
            frame = json.loads(device.readline())
        except (ValueError, UnicodeError):
            continue
        if frame.get('type') == 'status': statuses.append(frame)
received = max([baseline] + [f['rx'] for f in statuses]) - baseline
result = dict(payload_bytes=len(payload)-1, sent=count, accepted=received, statuses=statuses)
Path('backups/usb-state-burst.json').write_text(json.dumps(result, indent=2))
print(json.dumps({k:v for k,v in result.items() if k != 'statuses'}))
assert received == count, 'USB dropped or corrupted valid host states'
print('PASS: every real USB state frame accepted during redraw burst')
