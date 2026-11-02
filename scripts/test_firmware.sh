#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
work="${TMPDIR:-$HOME/.cache}/embeddeck-firmware-tests"
mkdir -p "$work"
g++ -std=c++17 -Wall -Wextra -Werror -Iinclude tests/button_test.cpp -o "$work/button-test"
"$work/button-test"
g++ -std=c++17 -Wall -Wextra -Werror -Iinclude \
  -I.pio/libdeps/hardware-test/ArduinoJson/src \
  tests/protocol_test.cpp -o "$work/protocol-test"
"$work/protocol-test"
g++ -std=c++17 -Wall -Wextra -Werror -Iinclude \
  -I.pio/libdeps/hardware-test/ArduinoJson/src \
  tests/float_state_test.cpp -o "$work/float-state-test"
"$work/float-state-test"
g++ -std=c++17 -Wall -Wextra -Werror -Iinclude tests/ui_layout_test.cpp -o "$work/ui-test"
"$work/ui-test"
g++ -std=c++17 -Wall -Wextra -Werror -Iinclude tests/float_ui_test.cpp -o "$work/float-ui-test"
"$work/float-ui-test"
g++ -std=c++17 -Wall -Wextra -Werror -Iinclude tests/gesture_test.cpp -o "$work/gesture-test"
"$work/gesture-test"
g++ -std=c++17 -Wall -Wextra -Werror -Iinclude tests/ring_feedback_test.cpp -o "$work/ring-test"
"$work/ring-test"
python3 tests/fixture_test.py
