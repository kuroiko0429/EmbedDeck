#pragma once
namespace WorkspaceUI {
// Landscape rotation 3 has the encoder on the right. Portrait 0 puts it above.
constexpr int kRotation = 0;
constexpr int kWidth = 170, kHeight = 320;
struct Rect { int x, y, w, h; };
constexpr Rect modeTab(int slot) {
    return {10 + slot * 38, 33, 36, 20};
}
constexpr Rect workspaceCell(int slot) {
    return {12 + (slot % 5) * 30, 184 + (slot / 5) * 29, 26, 24};
}
constexpr int firstWorkspace(int current) {
    return current > 0 ? ((current - 1) / 10) * 10 + 1 : 1;
}
} // namespace WorkspaceUI
