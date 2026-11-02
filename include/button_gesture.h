#pragma once
#include <cstdint>
#include "debounced_button.h"

// Feed debounced state every loop. A hold consumes the eventual release.
class ButtonGesture {
public:
    enum Event { None, Click, Hold };
    void suppress() { blocked_ = true; releaseTracking_ = false; }
    void observeRaw(bool down, uint32_t now) {
        if (!blocked_) return;
        if (down) { releaseTracking_ = false; return; }
        if (!releaseTracking_) { releasedAt_ = now; releaseTracking_ = true; }
        if (uint32_t(now - releasedAt_) >= 20) {
            blocked_ = false; down_ = false; held_ = false;
        }
    }
    Event update(bool down, uint32_t now) {
        observeRaw(down, now);
        if (blocked_) { down_ = false; return None; }
        if (down && !down_) { started_ = now; held_ = false; }
        Event event = None;
        if (down_ && !held_ && uint32_t(now - started_) >= 600) {
            held_ = true;
            event = Hold;
        }
        if (!down && down_ && !held_) event = Click;
        down_ = down;
        return event;
    }
private:
    bool down_ = false, held_ = false, blocked_ = false, releaseTracking_ = false;
    uint32_t started_ = 0, releasedAt_ = 0;
};

inline ButtonGesture::Event readGesture(DebouncedButton& debounce, ButtonGesture& gesture,
                                        bool raw, bool linked, uint32_t now) {
    debounce.update(raw, now);
    if (!linked) gesture.suppress();
    gesture.observeRaw(raw, now);
    // Don't clear an offline block on an uncommitted raw press/release edge.
    if (raw != debounce.pressed()) return ButtonGesture::None;
    return gesture.update(debounce.pressed(), now);
}
