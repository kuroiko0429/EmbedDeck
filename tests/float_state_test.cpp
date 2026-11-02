#include <cassert>
#include <cstdio>
#include <string>
#include "usb_protocol.h"

int main() {
    const char* raw = R"({"v":3,"type":"state","workspace":2,"name":"2","workspaces":[2],"backend":"hyprland","connected":true,"error":"","mode":"float","focus_axis":"horizontal","width_mode":"fine","layout":"dwindle","app":"kitty","window_width":600,"width_percent":32,"float_action":"move_x","window_floating":true,"window_ready":true,"window_x":-1920,"window_y":40,"window_height":400})";
    DynamicJsonDocument doc(8192);
    assert(!deserializeJson(doc, raw));
    auto encoded = [&]() { std::string s; serializeJson(doc, s); return s; };
    HostState state;
    for (const char* action : {"move_x", "move_y", "resize_w", "resize_h"}) {
        doc["float_action"] = action;
        assert(applyState(encoded().c_str(), state, 10));
        assert(!strcmp(state.mode, "float") && !strcmp(state.floatAction, action));
        assert(state.windowReady && state.windowFloating && state.windowX == -1920);
    }
    auto invalid = [&](const char* field, auto bad) {
        JsonVariant v = doc[field];
        const std::string previous = encoded();
        v.set(bad);
        assert(!applyState(encoded().c_str(), state, 20));
        assert(state.lastSeen == 10 && state.windowX == -1920);
        assert(!deserializeJson(doc, previous));
    };
    invalid("float_action", "shell");
    invalid("window_ready", "true");
    invalid("window_floating", 1);
    invalid("window_x", -100001);
    invalid("window_y", 100001);
    invalid("window_height", -1);
    invalid("window_width", 0);
    invalid("window_height", 0);
    doc.remove("float_action");
    assert(!applyState(encoded().c_str(), state, 20));
    assert(!deserializeJson(doc, raw));
    // Largest documented strings and occupied-workspace list must still fit
    // the wire cap and the firmware document, including native 64-bit tests.
    doc["name"] = std::string(63, 'n');
    doc["error"] = std::string(95, 'e');
    doc["app"] = std::string(24, 'a');
    doc["layout"] = std::string(31, 'l');
    JsonArray occupied = doc["workspaces"].to<JsonArray>();
    for (int id = 9900; id < 9932; ++id) occupied.add(id);
    const auto largest = encoded();
    assert(largest.size() <= 1023);
    assert(applyState(largest.c_str(), state, 30));
    assert(state.count == 32);
    puts("PASS: FLOAT action/geometry schema, atomic rejection, maximum v3 state");
}
