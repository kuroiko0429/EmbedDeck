#!/usr/bin/env python3
"""Synthetic v3 device -> actual Go controller/IPC, using owned windows only.

This is NOT a physical USB/button test. Stop the normal USB daemon first.
"""
import json
import math
from pathlib import Path
from check_daemon_modes import Device, focus
from check_hyprscrolling import dispatch, fixture, query, wait_for


def main():
    observations = []
    with fixture() as (workspace, windows):
        address = windows[0]
        selector = json.dumps('address:' + address)
        focus(address)

        def window():
            return next(w for w in query('clients') if w['address'] == address)

        def native(name, fields):
            dispatch('hl.dsp.window.' + name + '({' + fields + ', window = ' + selector + '})')

        device = Device()
        try:
            device.state(v=3, mode='workspace', workspace=workspace)
            for expected in ('focus', 'width', 'float'):
                device.button('user'); device.state(mode=expected)
            device.state(float_action='move_x', window_ready=True, window_floating=False)
            before = window()
            device.action('encoder', delta=1, ok=False)
            assert window()['at'] == before['at'] and window()['size'] == before['size']
            observations.append(dict(test='tiled_edit_guard', floating=False))
            device.button('knob', 'hold')
            wait_for(lambda: window()['floating'])
            native('resize', 'x = 600, y = 400, relative = false')
            monitor = next(m for m in query('monitors') if m['id'] == window()['monitor'])
            width, height = monitor['width'], monitor['height']
            if monitor['transform'] % 2:
                width, height = height, width
            reserved = monitor['reserved']
            left = math.ceil(monitor['x'] + reserved[0])
            top = math.ceil(monitor['y'] + reserved[1])
            right = math.floor(monitor['x'] + width / monitor['scale'] - reserved[2])
            bottom = math.floor(monitor['y'] + height / monitor['scale'] - reserved[3])
            native('move', f'x = {left + 100}, y = {top + 100}, relative = false')
            wait_for(lambda: window()['at'] == [left + 100, top + 100])

            def confirmed(action):
                focus(address)
                w = window()
                s = device.state(mode='float', float_action=action, window_ready=True, window_floating=True,
                                 window_x=w['at'][0], window_y=w['at'][1],
                                 window_width=w['size'][0], window_height=w['size'][1])
                return w, s

            for action, field, index in [('move_x', 'at', 0), ('move_y', 'at', 1),
                                         ('resize_w', 'size', 0), ('resize_h', 'size', 1)]:
                w, _ = confirmed(action)
                start = w[field][index]
                device.action('encoder', delta=2)
                wait_for(lambda: window()[field][index] == start + 20)
                w, state = confirmed(action)
                observations.append(dict(test=action, before=start, actual=w[field][index],
                                         readback={k: state[k] for k in ('window_x', 'window_y', 'window_width', 'window_height')}))
                device.action('encoder', delta=-2)
                wait_for(lambda: window()[field][index] == start)
                device.button('knob')
            confirmed('move_x')
            # Hyprland's native resize recenters the window. The daemon must
            # restore its anchor, including exactly on the left/top boundary.
            native('move', f'x = {left}, y = {top}, relative = false')
            wait_for(lambda: window()['at'] == [left, top])
            device.button('knob'); device.button('knob')
            confirmed('resize_w'); device.action('encoder', delta=2)
            wait_for(lambda: window()['size'] == [620, 400])
            assert window()['at'] == [left, top]
            device.button('knob'); confirmed('resize_h')
            device.action('encoder', delta=2)
            wait_for(lambda: window()['size'] == [620, 420])
            assert window()['at'] == [left, top]
            observations.append(dict(test='resize_anchor_at_left_top', actual=window()['at'], size=window()['size']))
            device.button('knob'); confirmed('move_x')

            def inside():
                w = window(); x, y = w['at']; w, h = w['size']
                assert left <= x and top <= y and x + w <= right and y + h <= bottom, (x, y, w, h)
                assert w >= min(160, right - left) and h >= min(100, bottom - top), (w, h)
                return [x, y, w, h]

            # Push each edge beyond the usable monitor: input remains bounded.
            counts = {'move_x': math.ceil((right - left) / 320) + 1,
                      'move_y': math.ceil((bottom - top) / 320) + 1}
            for action in ('move_x', 'move_y', 'resize_w', 'resize_h'):
                confirmed(action)
                for delta in (-32, 32):
                    repeat = counts['move_y' if action in ('move_y', 'resize_h') else 'move_x']
                    for _ in range(repeat):
                        focus(address); device.action('encoder', delta=delta)
                    geometry = inside()
                    observations.append(dict(test='bounds_' + action, direction=delta, actual=geometry))
                device.button('knob')
            # A fullscreen target must neither move nor change floating state.
            native('fullscreen', 'mode = "fullscreen", action = "set"')
            wait_for(lambda: window()['fullscreen'] != 0)
            focus(address)
            device.state(mode='float', window_ready=False)
            device.action('encoder', delta=1, ok=False)
            device.button('knob', 'hold', ok=False)
            assert window()['floating'] and window()['fullscreen'] != 0
            native('fullscreen', 'mode = "fullscreen", action = "unset"')
            wait_for(lambda: window()['fullscreen'] == 0)
            focus(address); device.state(window_ready=True)
            device.button('knob', 'hold')
            wait_for(lambda: not window()['floating'])
            device.state(mode='float', window_floating=False, window_ready=True)
            device.button('user', 'hold'); device.state(mode='workspace', workspace=workspace)
            assert query('activeworkspace')['id'] == workspace
            observations.append(dict(test='fullscreen_guard_toggle_back_and_escape', floating=False, workspace=workspace))
        finally:
            device.close()
    Path('backups/float-daemon-check.json').write_text(json.dumps(observations, indent=2))
    print(json.dumps(observations, indent=2))
    print('PASS: v3 device -> real Go -> Hyprland floating toggle, X/Y/W/H, boundaries and fullscreen guard; desktop restored')


if __name__ == '__main__':
    main()
