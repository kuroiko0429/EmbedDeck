#include <Arduino.h>
#include <TFT_eSPI.h>
#include <RotaryEncoder.h>
#include "debounced_button.h"
#include "usb_protocol.h"
#include "ui_layout.h"

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
    label("EMBEDDECK", 10, 12, kFg, 2);
    ui.fillCircle(14, 38, 3, accent);
    label(online ? "HYPRLAND / USB" : "WAITING FOR HOST", 26, 34, accent);
    ui.drawFastHLine(10, 51, 150, kPanel);

    ui.fillRoundRect(10, 61, 150, 99, 8, kPanel);
    label("WORKSPACE", 19, 70, kMuted);
    char number[16];
    if (online) snprintf(number, sizeof(number), "%02d", host.workspace);
    else snprintf(number, sizeof(number), "--");
    const int numberSize = strlen(number) > 3 ? 4 : (strlen(number) > 2 ? 6 : 7);
    ui.setTextSize(numberSize); ui.setTextDatum(MC_DATUM); ui.setTextColor(accent);
    ui.drawString(number, 85, 117, 1);

    char hint[25];
    if (online) snprintf(hint, sizeof(hint), "%.24s", activity);
    else if (recent && host.error[0]) snprintf(hint, sizeof(hint), "%.24s", host.error);
    else snprintf(hint, sizeof(hint), "Start embeddeck daemon");
    // The bundled font is ASCII only; never render UTF-8 bytes as random glyphs.
    for (char* p = hint; *p; ++p) if ((unsigned char)*p < 32 || (unsigned char)*p > 126) *p = '?';
    label(hint, 12, 169, kMuted);

    const int first = WorkspaceUI::firstWorkspace(online ? host.workspace : 1);
    for (int slot = 0; slot < 10; ++slot) {
        const int id = first + slot;
        const auto cell = WorkspaceUI::workspaceCell(slot);
        const bool active = online && host.workspace == id;
        ui.fillRoundRect(cell.x, cell.y, cell.w, cell.h, 4, active ? kGreen : kPanel);
        ui.setTextSize(1); ui.setTextDatum(MC_DATUM);
        ui.setTextColor(active ? kBg : (host.occupied(id) && online ? kFg : kMuted));
        char item[12]; snprintf(item, sizeof(item), "%d", id);
        ui.drawString(item, cell.x + cell.w / 2, cell.y + 11, 1);
        if (!active && host.occupied(id) && online) ui.drawFastHLine(cell.x + 8, cell.y + 21, 10, kGreen);
    }
    ui.drawFastHLine(10, 249, 150, kPanel);
    label("TURN", 12, 259, kGreen); label("prev / next", 53, 259);
    label("PRESS", 12, 275, kGreen); label("last workspace", 53, 275);
    label("USER", 12, 291, kGreen); label("workspace 1", 53, 291);
    label(online ? "LIVE / synced" : "OFFLINE / no actions", 12, 311, accent);
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
    display.init(); display.setRotation(WorkspaceUI::kRotation);
    pinMode(TFT_BL, OUTPUT); digitalWrite(TFT_BL, HIGH);
    display.fillScreen(kBg);
    ui.setColorDepth(16);
    spriteReady = ui.createSprite(WorkspaceUI::kWidth, WorkspaceUI::kHeight) != nullptr;
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
