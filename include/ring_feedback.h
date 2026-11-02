#pragma once
#include <array>
#include <cstdint>

namespace RingFeedback {
constexpr int kCount = 8;
constexpr uint32_t kTailMs = 180;
struct RGB {
    uint8_t r, g, b;
    constexpr bool operator==(RGB other) const { return r == other.r && g == other.g && b == other.b; }
    constexpr bool operator!=(RGB other) const { return !(*this == other); }
};
constexpr RGB kOff{0, 0, 0}, kGreen{55, 210, 70}, kBlue{50, 105, 245}, kYellow{245, 180, 20}, kOffline{48, 20, 0};
constexpr RGB kMagenta{211, 134, 155};
using Frame = std::array<RGB, kCount>;
enum class Mode { Workspace, Focus, Width, Floating };

inline RGB scaled(RGB color, unsigned level) {
    return {uint8_t(unsigned(color.r) * level / 255), uint8_t(unsigned(color.g) * level / 255), uint8_t(unsigned(color.b) * level / 255)};
}

class Ring {
public:
    // Visual cycle: one decoder step per LED, NOT a calibrated physical angle.
    // Normalize before addition, so even INT64_MIN is safe without abs/negation.
    void advance(int64_t delta, uint32_t now) {
        if (!delta) return;
        head_ = (head_ + int(delta % kCount) + kCount) % kCount;
        direction_ = delta > 0 ? 1 : -1;
        lastTurn_ = now; turned_ = true;
    }
    int head() const { return head_; }
    Frame render(Mode mode, int percent, bool available, uint32_t now) const {
        Frame frame{};
        if (!available) { frame[0] = kOffline; return frame; }
        if (mode == Mode::Width) {
            const int bounded = percent < 0 ? 0 : (percent > 100 ? 100 : percent);
            for (int i = 0; i < kCount; ++i) {
                const int fill = bounded * kCount - i * 100;
                const unsigned level = fill <= 0 ? 0 : (fill >= 100 ? 255 : unsigned(fill * 255 / 100));
                frame[i] = scaled(kYellow, level);
            }
            return frame;
        }
        const RGB color = mode == Mode::Floating ? kMagenta : (mode == Mode::Focus ? kBlue : kGreen);
        frame[head_] = color;
        const uint32_t age = now - lastTurn_;
        if (turned_ && age < kTailMs) {
            frame[(head_ - direction_ + kCount) % kCount] = scaled(color, (kTailMs - age) * 90 / kTailMs);
            frame[(head_ - direction_ * 2 + kCount) % kCount] = scaled(color, (kTailMs - age) * 30 / kTailMs);
        }
        return frame;
    }
private:
    int head_ = 0, direction_ = 1;
    uint32_t lastTurn_ = 0;
    bool turned_ = false;
};
} // namespace RingFeedback
