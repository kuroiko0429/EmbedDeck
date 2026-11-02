#!/usr/bin/env python3
"""Hold an isolated two-window workspace for human device testing, then clean up."""
import argparse
from pathlib import Path
import signal
import time
from check_hyprscrolling import fixture

parser = argparse.ArgumentParser()
parser.add_argument('--workspace', type=int, default=9)
parser.add_argument('--seconds', type=float, default=300)
parser.add_argument('--stop-file', required=True)
args = parser.parse_args()
stop = Path(args.stop_file)
if stop.exists():
    raise SystemExit('Stop sentinel already exists; choose a fresh path')
interrupted = False

def finish(*_):
    global interrupted
    interrupted = True

signal.signal(signal.SIGTERM, finish)
signal.signal(signal.SIGINT, finish)
with fixture(preferred=args.workspace) as (workspace, windows):
    print(f'READY workspace={workspace} windows={windows}', flush=True)
    end = time.monotonic() + args.seconds
    while time.monotonic() < end and not stop.exists() and not interrupted:
        time.sleep(.2)
print('CLEANED: fixture windows closed and original workspace/focus restored', flush=True)
