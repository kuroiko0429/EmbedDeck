#include <cassert>
#include <cstdio>
#include "debounced_button.h"
int main() {
    DebouncedButton b;
    assert(!b.update(false, 0));
    assert(!b.update(true, 10));
    assert(!b.update(false, 12));
    assert(!b.update(true, 15));
    assert(!b.update(true, 34));
    assert(b.update(true, 35));
    assert(b.pressed());
    assert(!b.update(true, 100));
    assert(b.presses() == 1);
    assert(!b.update(false, 101));
    assert(b.update(false, 121));
    assert(!b.pressed());
    assert(b.presses() == 1);
    DebouncedButton wrap;
    assert(!wrap.update(true, 0xfffffff0U));
    assert(wrap.update(true, 4));
    puts("PASS: debounce, hold, release, press count, millis wrap");
}
