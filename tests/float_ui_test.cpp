#include <cassert>
#include <cstring>
#include <cstdio>
#include "float_ui.h"
#include "ui_layout.h"

int main() {
    char value[16];
    const char* actions[] = {"move_x", "move_y", "resize_w", "resize_h"};
    const char* labels[] = {"MOVE X", "MOVE Y", "WIDTH", "HEIGHT"};
    const char* values[] = {"-1920", "40", "600", "400"};
    for (int i = 0; i < 4; ++i) {
        assert(!strcmp(FloatUI::actionLabel(actions[i]), labels[i]));
        FloatUI::formatValue(value, sizeof(value), actions[i], -1920, 40, 600, 400, true, true);
        assert(!strcmp(value, values[i]));
        const auto tab = WorkspaceUI::modeTab(i);
        assert(tab.x >= 0 && tab.y >= 0 && tab.x + tab.w <= WorkspaceUI::kWidth);
        assert(tab.w >= 30 && tab.y + tab.h <= WorkspaceUI::kHeight);
        if (i) assert(WorkspaceUI::modeTab(i - 1).x + tab.w < tab.x);
    }
    FloatUI::formatValue(value, sizeof(value), "move_x", -100000, 100000, 100000, 100000, true, true);
    assert(!strcmp(value, "-100000"));
    assert(int(strlen(value)) * 6 * FloatUI::valueSize(value) <= 134);
    FloatUI::formatValue(value, sizeof(value), "move_x", 0, 0, 0, 0, false, true);
    assert(!strcmp(value, "--"));
    FloatUI::formatValue(value, sizeof(value), "move_x", 0, 0, 600, 400, true, false);
    assert(!strcmp(value, "TILED"));
    char tiny[2];
    FloatUI::formatValue(tiny, sizeof(tiny), "move_x", -100000, 0, 0, 0, true, true);
    assert(tiny[1] == '\0');
    puts("PASS: FLOAT labels, actual geometry values, tiled/unavailable hints, bounded text and four tabs");
}
