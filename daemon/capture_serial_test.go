package main

import (
	"os/exec"
	"testing"
)

// Exercise the actual capture script with Python's stdlib unittest and a fake
// serial module. No pyserial installation or real device is needed.
func TestCaptureSerialSignals(t *testing.T) {
	cmd := exec.Command("python3", "-c", `
import contextlib
import io
from pathlib import Path
import runpy
import sys
import tempfile
import types
import unittest
from unittest.mock import patch

class CaptureSerialTest(unittest.TestCase):
    def test_signals_are_low_before_open_and_capture_closes(self):
        testcase = self
        events = []

        class Serial:
            def __init__(self, port=None, baudrate=9600, timeout=None):
                self.dtr = True
                self.rts = True
                self.port = port
                self.is_open = False
                events.append(('construct', port, baudrate, timeout))
                if port is not None:
                    self.open()

            def open(self):
                events.append(('open', self.port, self.dtr, self.rts))
                testcase.assertFalse(self.dtr, 'DTR must be low before opening')
                testcase.assertFalse(self.rts, 'RTS must be low before opening')
                self.is_open = True

            def __enter__(self):
                return self

            def __exit__(self, *args):
                events.append(('close', self.is_open))
                self.is_open = False

            def readline(self):
                testcase.assertTrue(self.is_open)
                return b'{"v":3,"type":"hello","firmware":"test"}\n'

        with tempfile.TemporaryDirectory() as tmp:
            output = Path(tmp) / 'capture.log'
            argv = ['capture_serial.py', '--port', '/mock/serial',
                    '--seconds', '1', '--output', str(output)]
            with patch.dict(sys.modules, {'serial': types.SimpleNamespace(Serial=Serial)}), \
                 patch.object(sys, 'argv', argv), \
                 patch('time.monotonic', side_effect=[0, 0, 2]), \
                 contextlib.redirect_stdout(io.StringIO()) as stdout:
                runpy.run_path('../scripts/capture_serial.py', run_name='__main__')
            self.assertEqual(output.read_text(), '{"v":3,"type":"hello","firmware":"test"}\n')
            self.assertEqual(stdout.getvalue(), output.read_text())
        self.assertEqual(events, [
            ('construct', None, 115200, 1),
            ('open', '/mock/serial', False, False),
            ('close', True),
        ])

unittest.main(argv=['capture_serial_regression'], verbosity=2)
`)
	output, err := cmd.CombinedOutput()
	t.Logf("Python regression output:\n%s", output)
	if err != nil {
		t.Fatalf("capture script regression: %v", err)
	}
}
