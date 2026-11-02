#pragma once
#include <ArduinoJson.h>
#include <cstddef>
#include <cstdint>
#include <cstdio>
#include <cstring>

// NDJSON: bounded frames and discard-until-newline overflow recovery.
template <size_t Capacity> class LineBuffer {
public:
    bool push(char c) {
        if (ready_) { length_ = 0; ready_ = false; }
        if (c == '\r') return false;
        if (c == '\n') {
            if (overflow_) { length_ = 0; overflow_ = false; return false; }
            buffer_[length_] = '\0';
            ready_ = length_ != 0;
            return ready_;
        }
        if (overflow_) return false;
        if (length_ >= Capacity - 1 || c == '\0') { overflow_ = true; return false; }
        buffer_[length_++] = c;
        return false;
    }
    const char* data() const { return buffer_; }
private:
    char buffer_[Capacity]{};
    size_t length_ = 0;
    bool overflow_ = false, ready_ = false;
};

struct HostState {
    int workspace = 0;
    char name[64]{};
    char error[96]{};
    bool connected = false, received = false;
    uint32_t lastSeen = 0;
    int workspaces[32]{};
    size_t count = 0;
    bool online(uint32_t now) const { return received && connected && uint32_t(now - lastSeen) < 3500; }
    bool occupied(int id) const {
        for (size_t i = 0; i < count; ++i) if (workspaces[i] == id) return true;
        return false;
    }
};

inline bool applyState(const char* line, HostState& state, uint32_t now) {
    StaticJsonDocument<2048> doc;
    if (deserializeJson(doc, line, DeserializationOption::NestingLimit(4))) return false;
    if (!doc["v"].is<int>() || doc["v"].as<int>() != 1 ||
        !doc["type"].is<const char*>() || strcmp(doc["type"], "state") != 0 ||
        !doc["workspace"].is<int>() || !doc["connected"].is<bool>() ||
        !doc["name"].is<const char*>() || !doc["backend"].is<const char*>() ||
        strcmp(doc["backend"], "hyprland") != 0 || !doc["workspaces"].is<JsonArray>()) return false;
    if (doc["connected"].as<bool>() && doc["workspace"].as<int>() < 1) return false;
    JsonArray occupied = doc["workspaces"].as<JsonArray>();
    if (occupied.size() > 32) return false;
    for (JsonVariant id : occupied) if (!id.is<int>()) return false;
    HostState next;
    next.workspace = doc["workspace"].as<int>();
    next.connected = doc["connected"].as<bool>();
    next.received = true;
    next.lastSeen = now;
    snprintf(next.name, sizeof(next.name), "%s", doc["name"].as<const char*>());
    snprintf(next.error, sizeof(next.error), "%s", doc["error"] | "");
    for (JsonVariant id : occupied) next.workspaces[next.count++] = id.as<int>();
    state = next;
    return true;
}
