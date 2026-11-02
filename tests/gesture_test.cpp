#include <cassert>
#include <cstdio>
#include <initializer_list>
#include "button_gesture.h"
#include "debounced_button.h"
int main() {
    ButtonGesture g;
    assert(g.update(false, 0) == ButtonGesture::None);
    assert(g.update(true, 10) == ButtonGesture::None);
    assert(g.update(false, 100) == ButtonGesture::Click);
    assert(g.update(false, 101) == ButtonGesture::None);
    assert(g.update(true, 200) == ButtonGesture::None);
    assert(g.update(true, 799) == ButtonGesture::None);
    assert(g.update(true, 800) == ButtonGesture::Hold);
    assert(g.update(true, 900) == ButtonGesture::None);
    assert(g.update(false, 1000) == ButtonGesture::None);
    assert(g.update(true, 1100) == ButtonGesture::None);
    assert(g.update(false, 1800) == ButtonGesture::Hold);
    ButtonGesture wrap;
    assert(wrap.update(true, 0xffffff00U) == ButtonGesture::None);
    assert(wrap.update(true, 0x158U) == ButtonGesture::Hold);
    assert(wrap.update(false, 0x200U) == ButtonGesture::None);
    ButtonGesture offline;
    offline.suppress();
    assert(offline.update(true, 0) == ButtonGesture::None);
    assert(offline.update(true, 1000) == ButtonGesture::None);
    assert(offline.update(false, 1100) == ButtonGesture::None);
    assert(offline.update(false, 1120) == ButtonGesture::None);
    assert(offline.update(true, 1200) == ButtonGesture::None);
    assert(offline.update(false, 1300) == ButtonGesture::Click);
    for (uint32_t reconnect : {10U, 100U}) {
        DebouncedButton debounce;
        ButtonGesture gesture;
        assert(readGesture(debounce, gesture, false, false, 0) == ButtonGesture::None);
        assert(readGesture(debounce, gesture, true, false, 1) == ButtonGesture::None);
        assert(readGesture(debounce, gesture, true, true, reconnect) == ButtonGesture::None);
        assert(readGesture(debounce, gesture, true, true, 150) == ButtonGesture::None);
        assert(readGesture(debounce, gesture, false, true, 200) == ButtonGesture::None);
        assert(readGesture(debounce, gesture, false, true, 220) == ButtonGesture::None);
        assert(readGesture(debounce, gesture, true, true, 300) == ButtonGesture::None);
        assert(readGesture(debounce, gesture, true, true, 320) == ButtonGesture::None);
        assert(readGesture(debounce, gesture, false, true, 400) == ButtonGesture::None);
        assert(readGesture(debounce, gesture, false, true, 420) == ButtonGesture::Click);
    }
    DebouncedButton bounceDebounce;
    ButtonGesture bounceGesture;
    struct Sample { uint32_t ms; bool raw, linked; };
    for (const auto sample : {Sample{0,false,false}, {1,true,false}, {10,true,true},
                              {11,false,true}, {12,true,true}, {32,true,true}, {200,false,true}, {220,false,true}}) {
        assert(readGesture(bounceDebounce, bounceGesture, sample.raw, sample.linked, sample.ms) == ButtonGesture::None);
    }
    assert(readGesture(bounceDebounce, bounceGesture, true, true, 300) == ButtonGesture::None);
    assert(readGesture(bounceDebounce, bounceGesture, true, true, 320) == ButtonGesture::None);
    assert(readGesture(bounceDebounce, bounceGesture, false, true, 400) == ButtonGesture::None);
    assert(readGesture(bounceDebounce, bounceGesture, false, true, 420) == ButtonGesture::Click);
    puts("PASS: gestures, rollover, offline/reconnect suppression including raw release bounce");
}
