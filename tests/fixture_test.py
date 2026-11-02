"""Unit tests for fixture cleanup; compositor/process calls are mocked."""
import json
from pathlib import Path
import re
import sys
import unittest
from unittest.mock import patch

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / 'scripts'))
import check_hyprscrolling as fixtures


class FixtureCleanupTest(unittest.TestCase):
    def exercise(self, preserve, leave):
        state = {'workspace': 2, 'clients': []}

        def query(name):
            return {'activeworkspace': {'id': state['workspace']}, 'activewindow': {},
                    'workspaces': [{'id': 2}], 'clients': list(state['clients'])}[name]

        def dispatch(code):
            state['workspace'] = int(re.search(r'workspace = (\d+)', code).group(1))

        class Process:
            def __init__(self, cmd, **_):
                self.client = {'class': cmd[cmd.index('--class') + 1],
                               'address': '0xabc', 'workspace': {'id': state['workspace']}}
                state['clients'].append(self.client)
                self.running = True
            def poll(self):
                return None if self.running else 0
            def terminate(self):
                state['clients'].remove(self.client)
                self.running = False
            def wait(self, **_):
                return 0

        with patch.object(fixtures, 'query', query), patch.object(fixtures, 'dispatch', dispatch), \
             patch.object(fixtures, 'socket_path', return_value=Path('/fake/instance/.socket.sock')), \
             patch.object(fixtures.shutil, 'which', return_value='/fake/kitty'), \
             patch.object(fixtures.subprocess, 'Popen', Process), \
             patch.object(fixtures.subprocess, 'check_output', return_value=json.dumps([
                 {'instance': 'instance', 'wl_socket': 'wayland-test'}])):
            with fixtures.fixture(preferred=9, preserve_navigation=preserve):
                if leave:
                    state['workspace'] = 3
        self.assertEqual(state['clients'], [])
        return state['workspace']

    def test_physical_session_preserves_user_navigation(self):
        self.assertEqual(self.exercise(preserve=True, leave=True), 3)

    def test_physical_session_restores_when_still_in_fixture(self):
        self.assertEqual(self.exercise(preserve=True, leave=False), 2)

    def test_automated_fixture_always_restores(self):
        self.assertEqual(self.exercise(preserve=False, leave=True), 2)


if __name__ == '__main__':
    unittest.main()
