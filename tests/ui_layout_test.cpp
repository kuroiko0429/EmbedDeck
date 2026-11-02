#include <cassert>
#include <cstdio>
#include "ui_layout.h"
int main() {
    using namespace WorkspaceUI;
    assert(kRotation == 0);
    assert(kWidth == 170 && kHeight == 320);
    for (int slot = 0; slot < 10; ++slot) {
        const Rect box = workspaceCell(slot);
        assert(box.x >= 0 && box.y >= 0);
        assert(box.x + box.w <= kWidth && box.y + box.h <= kHeight);
        for (int other = 0; other < slot; ++other) {
            const Rect b = workspaceCell(other);
            assert(box.x >= b.x+b.w || b.x >= box.x+box.w || box.y >= b.y+b.h || b.y >= box.y+box.h);
        }
    }
    assert(workspaceCell(0).y == workspaceCell(4).y);
    assert(workspaceCell(5).y > workspaceCell(0).y);
    assert(firstWorkspace(1) == 1);
    assert(firstWorkspace(10) == 1);
    assert(firstWorkspace(11) == 11);
    assert(firstWorkspace(99) == 91);
    puts("PASS: portrait 170x320, knob-top rotation, 5x2 grid bounds and page selection");
}
