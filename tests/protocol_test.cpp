#include <cassert>
#include <cstring>
#include <cstdio>
#include <string>
#include "usb_protocol.h"
int main() {
    const char* valid = R"({"v":2,"type":"state","workspace":2,"name":"2","workspaces":[1,2,4],"backend":"hyprland","connected":true,"error":"","mode":"width","focus_axis":"horizontal","width_mode":"preset","layout":"hyprscrolling","app":"kitty","window_width":946,"width_percent":50})";
    HostState state;
    assert(!state.online(0));
    assert(applyState(valid, state, 100));
    assert(state.workspace == 2 && state.occupied(4) && !state.occupied(3));
    assert(strcmp(state.mode, "width") == 0 && strcmp(state.widthMode, "preset") == 0);
    assert(state.windowWidth == 946 && state.widthPercent == 50);
    assert(strcmp(state.app, "kitty") == 0);
    assert(state.online(3599)); assert(!state.online(3600));
    std::string bad = valid; bad.replace(bad.find("\"v\":2"), 5, "\"v\":1");
    assert(!applyState(bad.c_str(), state, 200));
    bad = valid; bad.replace(bad.find("\"width\""), 7, "\"arbitrary\"");
    assert(!applyState(bad.c_str(), state, 200));
    bad = valid; bad.replace(bad.find("\"horizontal\""), 12, "\"diagonal\"");
    assert(!applyState(bad.c_str(), state, 200));
    bad = valid; bad.replace(bad.find("\"workspace\":2"), 13, "\"workspace\":2147483647");
    assert(!applyState(bad.c_str(), state, 200));
    assert(state.lastSeen == 100 && state.workspace == 2);
    assert(!applyState("garbage", state, 200));
    assert(!applyState(R"({"v":2,"type":"state","workspace":3})", state, 200));
    bad = valid; bad.replace(bad.find("\"connected\":true"), 16, "\"connected\":false");
    assert(applyState(bad.c_str(), state, 200)); assert(!state.online(200));
    assert(state.linkAlive(200)); assert(!state.linkAlive(3700));
    LineBuffer<8> line;
    assert(!line.push('{')); assert(!line.push('}')); assert(line.push('\n'));
    assert(std::strcmp(line.data(), "{}") == 0);
    for (int i=0;i<20;++i) assert(!line.push('x'));
    assert(!line.push('\n'));
    assert(!line.push('O')); assert(!line.push('K')); assert(!line.push('\r'));
    assert(line.push('\n')); assert(std::strcmp(line.data(), "OK") == 0);
    puts("PASS: v2 mode/axis/width state, reject v1/invalid enum, heartbeat, overflow recovery");
}
