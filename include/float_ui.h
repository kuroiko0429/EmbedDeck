#pragma once
#include <cstddef>
#include <cstdio>
#include <cstring>

namespace FloatUI {
inline const char* actionLabel(const char* action) {
    if (!std::strcmp(action, "move_y")) return "MOVE Y";
    if (!std::strcmp(action, "resize_w")) return "WIDTH";
    if (!std::strcmp(action, "resize_h")) return "HEIGHT";
    return "MOVE X";
}
inline void formatValue(char* text, size_t size, const char* action,
                        int x, int y, int width, int height, bool ready, bool floating) {
    if (!ready) { std::snprintf(text, size, "--"); return; }
    if (!floating) { std::snprintf(text, size, "TILED"); return; }
    const int value = !std::strcmp(action, "move_y") ? y :
        (!std::strcmp(action, "resize_w") ? width : (!std::strcmp(action, "resize_h") ? height : x));
    std::snprintf(text, size, "%d", value);
}
inline int valueSize(const char* text) {
    const size_t length = std::strlen(text);
    return length > 5 ? 2 : (length > 3 ? 3 : (length > 2 ? 4 : 6));
}
} // namespace FloatUI
