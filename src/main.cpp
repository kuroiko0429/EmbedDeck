#include <Arduino.h>
#include <TFT_eSPI.h>
#include <RotaryEncoder.h>
#include "debounced_button.h"
#include "usb_protocol.h"

namespace {
constexpr int kPower = 15, kEncoderA = 4, kEncoderB = 5;
constexpr int kEncoderKey = 0, kUserKey = 6;
constexpr uint16_t kBg = 0x2945, kPanel = 0x39c7, kFg = 0xef5b;
constexpr uint16_t kMuted = 0xa514, kGreen = 0xb5a8, kYellow = 0xf60c, kRed = 0xf2a9;
TFT_eSPI display;
TFT_eSprite ui(&display);
RotaryEncoder encoder(kEncoderA, kEncoderB, RotaryEncoder::LatchMode::TWO03);
DebouncedButton encoderButton, userButton;
portMUX_TYPE encoderMux = portMUX_INITIALIZER_UNLOCKED;
HostState host;
LineBuffer<1024> incoming;
uint32_t seq = 0, rx = 0;
long position = 0;
bool dirty = true, spriteReady = false;
char activity[40] = "Waiting for host daemon";

void IRAM_ATTR tickEncoder() {
    portENTER_CRITICAL_ISR(&encoderMux);
    encoder.tick();
    portEXIT_CRITICAL_ISR(&encoderMux);
}

template <size_t Size> void send(const StaticJsonDocument<Size>& doc) {
    // Never let a disconnected/slow host stall input processing.
    if (!Serial || Serial.availableForWrite() < int(measureJson(doc) + 1)) return;
    serializeJson(doc, Serial);
    Serial.write('\n');
}

void hello() {
    StaticJsonDocument<128> doc;
    doc["v"] = 1; doc["type"] = "hello"; doc["firmware"] = "0.2.0";
    send(doc);
}

void status() {
    StaticJsonDocument<192> doc;
    doc["v"] = 1; doc["type"] = "status";
    doc["workspace"] = host.workspace;
    doc["connected"] = host.online(millis()); doc["rx"] = rx;
    send(doc);
}

void sendEncoder(int delta) {
    StaticJsonDocument<128> doc;
    doc["v"] = 1; doc["type"] = "encoder"; doc["seq"] = ++seq; doc["delta"] = delta;
    send(doc);
}

void sendButton(const char* key, bool down) {
    StaticJsonDocument<192> doc;
    doc["v"] = 1; doc["type"] = "button"; doc["seq"] = ++seq;
    doc["button"] = key; doc["action"] = down ? "press" : "release";
    send(doc);
}

void label(const char* text, int x, int y, uint16_t color = kFg, int size = 1) {
    ui.setTextDatum(TL_DATUM);
    ui.setTextColor(color);
    ui.setTextSize(size);
    ui.drawString(text, x, y, 1);
}

void render() {
    if (!spriteReady) return;
    const bool online = host.online(millis());
    const bool recent = host.received && uint32_t(millis() - host.lastSeen) < 3500;
    const uint16_t accent = online ? kGreen : kYellow;
    ui.fillSprite(kBg);
    label("EMBEDDECK", 10, 9, kFg, 2);
    ui.fillCircle(229, 13, 3, accent);
    label(online ? "HYPRLAND" : "OFFLINE", 239, 9, accent);
    ui.drawFastHLine(10, 29, 300, kPanel);

    ui.fillRoundRect(10, 38, 110, 78, 7, kPanel);
    label("WORKSPACE", 19, 45, kMuted);
    char number[16];
    if (online) snprintf(number, sizeof(number), "%02d", host.workspace);
    else snprintf(number, sizeof(number), "--");
    label(number, 21, 62, accent, strlen(number) > 2 ? 4 : 6);

    label(online ? "DESKTOP CONTROL" : "USB CDC / JSON", 133, 42, accent);
    label("TURN   prev / next", 133, 61);
    label("PRESS  last workspace", 133, 77);
    label("USER   workspace 1", 133, 93);
    char hint[36];
    if (online) snprintf(hint, sizeof(hint), "%s", activity);
    else if (recent && host.error[0]) snprintf(hint, sizeof(hint), "%.31s", host.error);
    else snprintf(hint, sizeof(hint), "Start embeddeck daemon");
    // The bundled font is ASCII only; never render UTF-8 bytes as random glyphs.
    for (char* p = hint; *p; ++p) if ((unsigned char)*p < 32 || (unsigned char)*p > 126) *p = '?';
    label(hint, 133, 109, kMuted);

    const int first = online && host.workspace > 10 ? ((host.workspace - 1) / 10) * 10 + 1 : 1;
    for (int slot = 0; slot < 10; ++slot) {
        const int id = first + slot, x = 10 + slot * 30;
        const bool active = online && host.workspace == id;
        ui.fillRoundRect(x, 132, 26, 23, 4, active ? kGreen : kPanel);
        ui.setTextSize(1); ui.setTextDatum(MC_DATUM);
        ui.setTextColor(active ? kBg : (host.occupied(id) && online ? kFg : kMuted));
        char item[12]; snprintf(item, sizeof(item), "%d", id);
        ui.drawString(item, x + 13, 143, 1);
        if (!active && host.occupied(id) && online) ui.drawFastHLine(x + 8, 152, 10, kGreen);
    }
    label("USB serial", 10, 161, kMuted);
    label(online ? "LIVE / synced" : "No actions while offline", 133, 161, accent);
    ui.pushSprite(0, 0);
}

void receive() {
    // Budget each loop, so malformed/flooded CDC input cannot starve buttons.
    for (int budget = 0; budget < 2048 && Serial.available(); ++budget) {
        if (!incoming.push(char(Serial.read()))) continue;
        HostState next = host;
        if (applyState(incoming.data(), next, millis())) {
            const bool changed = !host.online(millis()) || next.workspace != host.workspace ||
                next.connected != host.connected || strcmp(next.error, host.error) != 0 ||
                next.count != host.count || memcmp(next.workspaces, host.workspaces, sizeof(host.workspaces)) != 0;
            host = next; ++rx;
            if (changed) { dirty = true; status(); }
        } else {
            StaticJsonDocument<256> doc;
            if (!deserializeJson(doc, incoming.data()) && doc["v"] == 1 &&
                doc["type"] == "ack" && doc["ok"].is<bool>() && !doc["ok"].as<bool>()) {
                snprintf(activity, sizeof(activity), "Host action failed");
                dirty = true;
            }
        }
    }
}
} // namespace

void setup() {
    for (int pin : {41, 12, 13, 44}) { pinMode(pin, OUTPUT); digitalWrite(pin, HIGH); }
    pinMode(kPower, OUTPUT); digitalWrite(kPower, HIGH);
    pinMode(2, OUTPUT); digitalWrite(2, LOW);
    pinMode(kEncoderKey, INPUT_PULLUP); pinMode(kUserKey, INPUT_PULLUP);
    pinMode(kEncoderA, INPUT_PULLUP); pinMode(kEncoderB, INPUT_PULLUP);
    Serial.begin(115200);
    attachInterrupt(digitalPinToInterrupt(kEncoderA), tickEncoder, CHANGE);
    attachInterrupt(digitalPinToInterrupt(kEncoderB), tickEncoder, CHANGE);
    display.init(); display.setRotation(3);
    pinMode(TFT_BL, OUTPUT); digitalWrite(TFT_BL, HIGH);
    display.fillScreen(kBg);
    ui.setColorDepth(16);
    spriteReady = ui.createSprite(320, 170) != nullptr;
    if (!spriteReady) {
        display.setTextColor(kRed, kBg); display.drawString("UI allocation failed", 10, 30, 2);
    }
    render(); hello();
}

void loop() {
    receive();
    const uint32_t now = millis();
    const bool online = host.online(now);
    static bool wasOnline = false;
    if (online != wasOnline) { dirty = true; wasOnline = online; }
    portENTER_CRITICAL(&encoderMux);
    const long next = encoder.getPosition();
    portEXIT_CRITICAL(&encoderMux);
    if (next != position) {
        long delta = next - position; position = next;
        snprintf(activity, sizeof(activity), delta > 0 ? "Next workspace >" : "< Previous workspace");
        if (online) while (delta) {
            const int chunk = delta > 32 ? 32 : (delta < -32 ? -32 : int(delta));
            sendEncoder(chunk); delta -= chunk;
        }
        dirty = true;
    }
    if (encoderButton.update(digitalRead(kEncoderKey) == LOW, now)) {
        if (online) sendButton("knob", encoderButton.pressed());
        if (encoderButton.pressed()) snprintf(activity, sizeof(activity), "Back to last workspace");
        dirty = true;
    }
    if (userButton.update(digitalRead(kUserKey) == LOW, now)) {
        if (online) sendButton("user", userButton.pressed());
        if (userButton.pressed()) snprintf(activity, sizeof(activity), "Home / workspace 1");
        dirty = true;
    }
    static uint32_t lastHello = 0, lastDraw = 0;
    if (uint32_t(now - lastHello) >= 2000) { hello(); lastHello = now; }
    if (dirty && uint32_t(now - lastDraw) >= 40) { render(); dirty = false; lastDraw = now; }
    delay(1);
}
