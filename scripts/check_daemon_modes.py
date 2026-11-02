#!/usr/bin/env python3
"""Synthetic device inputs over PTY -> real Go daemon -> real Hyprland.

This does not establish physical-button delivery. Creates only owned fixture
windows and restores the original desktop. Stop the USB daemon first.
"""
import json
import os
from pathlib import Path
import pty
import select
import signal
import subprocess
import time
import tty
from check_hyprscrolling import fixture, query, dispatch, layout, wait_for


class Device:
    def __init__(self):
        self.master, self.slave = pty.openpty()
        tty.setraw(self.slave)
        self.log = Path('backups/daemon-mode-check.log').open('w')
        env = dict(os.environ, EMBEDDECK_TEST_PTY=os.ttyname(self.slave))
        self.process = subprocess.Popen(['go', 'test', '-run', '^TestLivePTYBridge$', '-count=1', '-timeout=45s', '-v', '.'],
                                        cwd='daemon', env=env, stdout=self.log, stderr=self.log, start_new_session=True)
        self.pending = bytearray()
        self.seq = 0
        self.frames = []

    def frame(self, predicate, timeout=5):
        deadline = time.monotonic() + timeout
        while time.monotonic() < deadline:
            while b'\n' in self.pending:
                line, _, tail = self.pending.partition(b'\n')
                self.pending = bytearray(tail)
                value = json.loads(line)
                self.frames.append(value)
                if predicate(value):
                    return value
            ready, _, _ = select.select([self.master], [], [], .1)
            if ready:
                data = os.read(self.master, 4096)
                if not data:
                    raise RuntimeError('Daemon PTY closed')
                self.pending.extend(data)
        raise TimeoutError('No expected daemon frame; inspect backups/daemon-mode-check.log')

    def state(self, **expected):
        return self.frame(lambda f: f.get('type') == 'state' and all(f.get(k) == v for k, v in expected.items()))

    def action(self, kind, ok=True, **fields):
        self.seq += 1
        payload = dict(v=3, type=kind, seq=self.seq, **fields)
        data = json.dumps(payload).encode() + b'\n'
        while data:
            written = os.write(self.master, data)
            data = data[written:]
        ack = self.frame(lambda f: f.get('type') == 'ack' and f.get('seq') == self.seq)
        assert ack['ok'] is ok, (payload, ack)
        return ack

    def button(self, button, action='click', **kw):
        return self.action('button', button=button, action=action, **kw)

    def close(self):
        os.killpg(self.process.pid, signal.SIGTERM)
        try:
            self.process.wait(timeout=4)
        except subprocess.TimeoutExpired:
            os.killpg(self.process.pid, signal.SIGKILL); self.process.wait(timeout=3)
        self.log.close()
        os.close(self.master); os.close(self.slave)
        Path('backups/daemon-mode-frames.json').write_text(json.dumps(self.frames, indent=2))


def focus(address):
    dispatch('hl.dsp.focus({ window = ' + json.dumps('address:' + address) + ' })')
    wait_for(lambda: query('activewindow').get('address') == address)


def main():
    results = []
    with fixture() as (workspace, windows):
        left, right = windows
        device = Device()
        try:
            initial = device.state(v=3, mode='workspace', workspace=workspace)
            assert initial['layout'] == 'hyprscrolling'
            device.button('user'); device.state(mode='focus')
            focus(left)
            device.action('encoder', delta=1)
            wait_for(lambda: query('activewindow')['address'] == right)
            device.action('encoder', delta=-1)
            wait_for(lambda: query('activewindow')['address'] == left)
            results.append(dict(test='focus_horizontal', left=left, right=right))
            device.button('knob'); device.state(focus_axis='vertical')
            focus(right); layout('movewindowto l'); focus(left)
            device.action('encoder', delta=1)
            wait_for(lambda: query('activewindow')['address'] == right)
            device.action('encoder', delta=-1)
            wait_for(lambda: query('activewindow')['address'] == left)
            device.button('knob', 'hold')
            wait_for(lambda: query('activewindow')['address'] == right)
            results.append(dict(test='focus_vertical_and_history', up=left, down=right))
            # Recreate separate columns using only the owned fixture windows.
            layout('movewindowto r'); focus(left)
            device.button('user'); device.state(mode='width', width_mode='fine')
            before = query('activewindow')['size'][0]
            device.action('encoder', delta=1)
            grown = wait_for(lambda: (w if (w := query('activewindow')['size'][0]) > before else None))
            device.action('encoder', delta=-1)
            wait_for(lambda: abs(query('activewindow')['size'][0] - before) <= 2)
            results.append(dict(test='width_fine', before=before, grown=grown))
            device.button('knob'); device.state(width_mode='preset')
            device.action('encoder', delta=1)
            preset = query('activewindow')['size'][0]
            device.action('encoder', delta=-1)
            reverted = query('activewindow')['size'][0]
            assert preset != reverted
            device.button('knob', 'hold')
            fitted = wait_for(lambda: (w if (w := query('activewindow')['size'][0]) > grown else None))
            results.append(dict(test='width_presets_and_fit', preset=preset, reverted=reverted, fitted=fitted))
            device.button('user', 'hold'); device.state(mode='workspace', workspace=workspace)
            results.append(dict(test='user_hold_changes_mode_only', workspace=query('activeworkspace')['id']))
            device.button('knob', 'hold'); device.state(workspace=1)
            assert query('activeworkspace')['id'] == 1
            results.append(dict(test='workspace_knob_hold', workspace=1))
            used = {w['id'] for w in query('workspaces')}
            empty = next(n for n in range(99, 89, -1) if n not in used)
            dispatch(f'hl.dsp.focus({{ workspace = {empty} }})')
            device.state(workspace=empty)
            device.button('user'); device.state(mode='focus')
            failure = device.action('encoder', delta=1, ok=False)
            assert query('activeworkspace')['id'] == empty and not query('activewindow').get('address')
            results.append(dict(test='empty_workspace_guard', ack=failure))
        finally:
            device.close()
    Path('backups/daemon-mode-results.json').write_text(json.dumps(results, indent=2))
    print(json.dumps(results, indent=2))
    print('PASS: synthetic device -> actual Go daemon -> actual Hyprland modes; original focus restored')


if __name__ == '__main__':
    main()
