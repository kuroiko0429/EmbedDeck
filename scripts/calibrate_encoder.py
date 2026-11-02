#!/usr/bin/env python3
"""Read actual encoder counts without forwarding any desktop actions.

Stop the daemon first. This script only sends live compositor snapshots back to
keep the device's USB session active; it never dispatches desktop commands.
"""
import argparse
import json
from pathlib import Path
import signal
import subprocess
import time
import serial

parser = argparse.ArgumentParser()
parser.add_argument('--stop-file', required=True)
parser.add_argument('--seconds', type=float, default=180)
parser.add_argument('--output', default='backups/encoder-calibration.json')
args = parser.parse_args()
stop = Path(args.stop_file)
if stop.exists():
    raise SystemExit('Use a fresh stop sentinel path')
state = json.loads(subprocess.check_output(['./daemon/embeddeck', '--probe'], text=True))
if not state.get('connected'):
    raise SystemExit('A live host snapshot is required')
payload = json.dumps(state, separators=(',', ':')).encode() + b'\n'
frames = []
interrupted = False

def finish(*_):
    global interrupted
    interrupted = True

signal.signal(signal.SIGTERM, finish)
signal.signal(signal.SIGINT, finish)
port = '/dev/serial/by-id/usb-Espressif_USB_JTAG_serial_debug_unit_30:ED:A0:34:56:4C-if00'
ready = False
with serial.Serial(port=None, baudrate=115200, timeout=.1) as device:
    device.dtr = False; device.rts = False; device.port = port; device.open()
    device.reset_input_buffer()
    end = time.monotonic() + args.seconds
    last = 0
    while time.monotonic() < end and not stop.exists() and not interrupted:
        if time.monotonic() - last >= .5:
            device.write(payload); last = time.monotonic()
        try:
            frame = json.loads(device.readline())
        except (ValueError, UnicodeError):
            continue
        if not ready and frame.get('type') == 'status':
            ready = True
            print('READY: rotate exactly one full turn; desktop actions are disabled', flush=True)
        if ready and frame.get('type') == 'encoder':
            frames.append(frame)
            print(f"delta={frame['delta']} total={sum(f['delta'] for f in frames)}", flush=True)
result = dict(ready=ready, positive=sum(max(0, f['delta']) for f in frames),
              negative=-sum(min(0, f['delta']) for f in frames), net=sum(f['delta'] for f in frames), events=frames)
Path(args.output).parent.mkdir(parents=True, exist_ok=True)
Path(args.output).write_text(json.dumps(result, indent=2))
print(json.dumps({k:v for k,v in result.items() if k != 'events'}), flush=True)
