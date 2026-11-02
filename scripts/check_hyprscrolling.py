#!/usr/bin/env python3
"""Reversible real IPC tests using only temporary, owned kitty windows."""
import contextlib
import json
import os
from pathlib import Path
import shutil
import socket
import subprocess
import time
import uuid


def socket_path():
    root = Path(os.environ.get('XDG_RUNTIME_DIR', f'/run/user/{os.getuid()}'))
    signature = os.environ.get('HYPRLAND_INSTANCE_SIGNATURE')
    paths = [root / 'hypr' / signature / '.socket.sock'] if signature else list((root / 'hypr').glob('*/.socket.sock'))
    if len(paths) != 1:
        raise RuntimeError('Select exactly one Hyprland instance')
    return paths[0]


def ipc(command):
    with socket.socket(socket.AF_UNIX) as conn:
        conn.settimeout(2)
        conn.connect(str(socket_path()))
        conn.sendall(command.encode())
        result = bytearray()
        while data := conn.recv(65536):
            result.extend(data)
    return result.decode()


def query(name):
    return json.loads(ipc('j/' + name))


def dispatch(code):
    result = ipc('dispatch ' + code).strip()
    if result != 'ok':
        raise RuntimeError(result)


def layout(message):
    dispatch('hl.dsp.layout(' + json.dumps(message) + ')')


def wait_for(predicate, timeout=6):
    end = time.monotonic() + timeout
    while time.monotonic() < end:
        found = predicate()
        if found:
            return found
        time.sleep(.08)
    raise TimeoutError('Real compositor state did not reach expected condition')


@contextlib.contextmanager
def fixture(preferred=None):
    if not shutil.which('kitty'):
        raise RuntimeError('kitty not installed')
    old_ws, old_window = query('activeworkspace'), query('activewindow')
    used = {w['id'] for w in query('workspaces')}
    workspace = preferred if preferred is not None else next((n for n in range(98, 89, -1) if n not in used), None)
    if workspace is None or workspace in used:
        raise RuntimeError('No isolated test workspace available')
    instances = json.loads(subprocess.check_output(['hyprctl', 'instances', '-j'], text=True))
    instance = next(x for x in instances if x['instance'] == socket_path().parent.name)
    env = dict(os.environ, WAYLAND_DISPLAY=instance['wl_socket'])
    classes = ['embeddeck-fixture-' + uuid.uuid4().hex[:10] for _ in range(2)]
    processes = []
    try:
        dispatch(f'hl.dsp.focus({{ workspace = {workspace} }})')
        for cls in classes:
            processes.append(subprocess.Popen(['kitty', '--class', cls, '--title', 'EmbedDeck temporary test',
                                               'sh', '-c', 'printf "EmbedDeck temporary test window\\n"; exec sleep 180'],
                                              env=env, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL))
            wait_for(lambda: any(c.get('class') == cls for c in query('clients')))
        windows = {c['class']: c for c in query('clients') if c.get('class') in classes}
        assert all(w['workspace']['id'] == workspace for w in windows.values())
        yield workspace, [windows[c]['address'] for c in classes]
    finally:
        for p in processes:
            if p.poll() is None:
                p.terminate()
                try:
                    p.wait(timeout=3)
                except subprocess.TimeoutExpired:
                    p.kill(); p.wait(timeout=3)
        dispatch(f'hl.dsp.focus({{ workspace = {old_ws["id"]} }})')
        address = old_window.get('address')
        if address and any(c.get('address') == address for c in query('clients')):
            dispatch('hl.dsp.focus({ window = ' + json.dumps('address:' + address) + ' })')
        wait_for(lambda: not any(c.get('class') in classes for c in query('clients')))


def main():
    observations = []
    with fixture() as (workspace, windows):
        left, right = windows
        dispatch('hl.dsp.focus({ window = ' + json.dumps('address:' + left) + ' })')
        before = query('activewindow')['size'][0]
        layout('colresize +0.05')
        grown = wait_for(lambda: (w if (w := query('activewindow')['size'][0]) > before else None))
        layout('colresize -0.05')
        wait_for(lambda: abs(query('activewindow')['size'][0] - before) <= 2)
        observations.append(dict(test='fine_width', before=before, grown=grown))
        layout('focus r'); wait_for(lambda: query('activewindow')['address'] == right)
        layout('focus l'); wait_for(lambda: query('activewindow')['address'] == left)
        observations.append(dict(test='horizontal_focus', left=left, right=right))
        layout('colresize +conf'); preset = query('activewindow')['size'][0]
        layout('fit active')
        fitted = wait_for(lambda: (w if (w := query('activewindow')['size'][0]) > grown else None))
        observations.append(dict(test='preset_and_fit', preset=preset, fitted=fitted))
        dispatch('hl.dsp.focus({ window = ' + json.dumps('address:' + right) + ' })')
        layout('movewindowto l')
        layout('focus u'); wait_for(lambda: query('activewindow')['address'] == left)
        layout('focus d'); wait_for(lambda: query('activewindow')['address'] == right)
        observations.append(dict(test='vertical_focus', up=left, down=right))
        layout('focusback'); wait_for(lambda: query('activewindow')['address'] == left)
        observations.append(dict(test='focus_history', address=left))
    Path('backups/plugin-ipc-check.json').write_text(json.dumps(observations, indent=2))
    print(json.dumps(observations, indent=2))
    print('PASS: actual hyprscrolling operations; temporary windows closed, original focus restored')


if __name__ == '__main__':
    main()
