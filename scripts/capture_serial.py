#!/usr/bin/env python3
"""Capture real device events without toggling DTR/RTS. Run using pio's Python."""
import argparse
import time
from pathlib import Path
import serial

parser = argparse.ArgumentParser()
parser.add_argument('--port', default='/dev/serial/by-id/usb-Espressif_USB_JTAG_serial_debug_unit_30:ED:A0:34:56:4C-if00')
parser.add_argument('--seconds', type=float, default=90)
parser.add_argument('--output', default='backups/hardware-input.log')
args = parser.parse_args()
path = Path(args.output)
path.parent.mkdir(parents=True, exist_ok=True)
with serial.Serial(port=None, baudrate=115200, timeout=1) as device, path.open('a') as log:
    device.dtr = False
    device.rts = False
    device.port = args.port
    device.open()
    end = time.monotonic() + args.seconds
    while time.monotonic() < end:
        line = device.readline().decode(errors='replace')
        if line:
            log.write(line)
            log.flush()
            print(line, end='', flush=True)
