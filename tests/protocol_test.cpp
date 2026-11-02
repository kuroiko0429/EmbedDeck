#include <cassert>
#include <cstring>
#include <cstdio>
#include "usb_protocol.h"
int main() {
    HostState state;
    assert(!state.online(0));
    assert(applyState(R"({"v":1,"type":"state","workspace":2,"name":"2","workspaces":[1,2,4],"backend":"hyprland","connected":true,"error":""})", state, 100));
    assert(state.workspace == 2 && state.occupied(4) && !state.occupied(3));
    assert(state.online(3599));
    assert(!state.online(3600));
    assert(!applyState(R"({"v":2,"type":"state","workspace":3})", state, 200));
    assert(state.workspace == 2 && state.lastSeen == 100);
    assert(!applyState(R"({"v":1,"type":"state","workspace":"3","connected":true,"workspaces":[],"name":"3","backend":"hyprland"})", state, 200));
    assert(!applyState("garbage", state, 200));
    assert(applyState(R"({"v":1,"type":"state","workspace":0,"name":"","workspaces":[],"backend":"hyprland","connected":false,"error":"No session"})", state, 200));
    assert(!state.online(200) && std::strcmp(state.error, "No session") == 0);
    LineBuffer<8> line;
    assert(!line.push('{')); assert(!line.push('}')); assert(line.push('\n'));
    assert(std::strcmp(line.data(), "{}") == 0);
    for (int i=0;i<20;++i) assert(!line.push('x'));
    assert(!line.push('\n')); // discard entire oversized frame
    assert(!line.push('O')); assert(!line.push('K')); assert(!line.push('\r'));
    assert(line.push('\n')); assert(std::strcmp(line.data(), "OK") == 0);
    puts("PASS: host state schema, heartbeat expiry, occupancy, bad frames, overflow recovery");
}
