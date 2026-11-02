#pragma once
#include <cstdint>

class DebouncedButton {
public:
    bool update(bool raw, uint32_t now) {
        if (raw != candidate_) {
            candidate_ = raw;
            changedAt_ = now;
        }
        if (stable_ == candidate_ || uint32_t(now - changedAt_) < 20) return false;
        stable_ = candidate_;
        if (stable_) ++presses_;
        return true;
    }
    bool pressed() const { return stable_; }
    uint32_t presses() const { return presses_; }
private:
    bool candidate_ = false;
    bool stable_ = false;
    uint32_t changedAt_ = 0;
    uint32_t presses_ = 0;
};
