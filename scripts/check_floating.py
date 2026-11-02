#!/usr/bin/env python3
"""Verify native float/move/resize dispatchers on owned temporary windows only."""
import json
from pathlib import Path
from check_hyprscrolling import dispatch, fixture, query, wait_for


def main():
    observations = []
    with fixture() as (workspace, windows):
        address = windows[0]
        selector = json.dumps('address:' + address)

        def window():
            return next(w for w in query('clients') if w['address'] == address)

        def call(name, values):
            dispatch('hl.dsp.window.' + name + '({' + values + ', window = ' + selector + '})')

        call('float', 'action = "enable"')
        wait_for(lambda: window()['floating'])
        call('resize', 'x = 600, y = 400, relative = false')
        wait_for(lambda: window()['size'] == [600, 400])
        monitor = next(m for m in query('monitors') if m['id'] == window()['monitor'])
        x, y = monitor['x'] + 100, monitor['y'] + 100
        call('move', f'x = {x}, y = {y}, relative = false')
        wait_for(lambda: window()['at'] == [x, y])
        for name, values, field, expected in [
            ('move', f'x = {x + 20}, y = {y}, relative = false', 'at', [x + 20, y]),
            ('move', f'x = {x + 20}, y = {y + 20}, relative = false', 'at', [x + 20, y + 20]),
            ('resize', 'x = 620, y = 400, relative = false', 'size', [620, 400]),
            ('resize', 'x = 620, y = 420, relative = false', 'size', [620, 420]),
        ]:
            call(name, values)
            wait_for(lambda: window()[field] == expected)
            observations.append(dict(dispatch=name, field=field, actual=window()[field]))
        call('float', 'action = "disable"')
        wait_for(lambda: not window()['floating'])
        observations.append(dict(dispatch='float', floating=window()['floating']))
    Path('backups/float-ipc-check.json').write_text(json.dumps(observations, indent=2))
    print(json.dumps(observations, indent=2))
    print('PASS: native floating toggle, X/Y movement and width/height resize; original desktop restored')


if __name__ == '__main__':
    main()
