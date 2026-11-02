#include <cassert>
#include <climits>
#include <cstdio>
#include "ring_feedback.h"

int main() {
    using namespace RingFeedback;
    Ring ring;
    auto f = ring.render(Mode::Workspace, 0, true, 0);
    assert(f[0] == kGreen);
    for (int i=1; i<kCount; ++i) assert(f[i] == kOff);
    ring.advance(1, 10);
    f = ring.render(Mode::Workspace, 0, true, 10);
    assert(f[1] == kGreen && f[0] != kOff);
    f = ring.render(Mode::Workspace, 0, true, 190);
    assert(f[0] == kOff && f[1] == kGreen);
    ring.advance(-1, 200);
    f = ring.render(Mode::Focus, 0, true, 200);
    assert(f[0] == kBlue && f[1] != kOff);
    ring.advance(-1, 210);
    assert(ring.head() == 7);
    ring.advance(8, 220); assert(ring.head() == 7);
    ring.advance(LLONG_MIN, 230); assert(ring.head() == 7);
    ring.advance(1, 0xfffffff0U);
    f = ring.render(Mode::Workspace, 0, true, 0x100U);
    assert(f[7] == kOff);
    f = ring.render(Mode::Width, 50, true, 0);
    for (int i=0; i<kCount; ++i) assert(f[i] == (i<4 ? kYellow : kOff));
    f = ring.render(Mode::Width, 100, true, 0);
    for (auto c : f) assert(c == kYellow);
    for (int percent : {-1, 0}) {
        f = ring.render(Mode::Width, percent, true, 0);
        for (auto c : f) assert(c == kOff);
    }
    f = ring.render(Mode::Width, 101, true, 0);
    for (auto c : f) assert(c == kYellow);
    f = ring.render(Mode::Width, 1, true, 0);
    assert(f[0] != kOff && f[0] != kYellow && f[1] == kOff);
    f = ring.render(Mode::Workspace, 0, false, 0);
    assert(f[0] == kOffline);
    for (int i=1; i<kCount; ++i) assert(f[i] == kOff);
    ring.advance(1, 500);
    f = ring.render(Mode::Floating, 0, true, 500);
    assert(f[ring.head()] == kMagenta);
    assert(f[(ring.head() + kCount - 1) % kCount] != kOff);
    f = ring.render(Mode::Floating, 0, true, 500 + kTailMs);
    for (int i=0; i<kCount; ++i) assert(f[i] == (i == ring.head() ? kMagenta : kOff));
    f = ring.render(Mode::Floating, 0, false, 700);
    assert(f[0] == kOffline);
    puts("PASS: ring phase/wrap, direction tail/expiry, mode colors, width bar, offline, integer/time limits");
}
