#include <Arduino.h>
#include <TFT_eSPI.h>
#include <RotaryEncoder.h>
#include <Adafruit_NeoPixel.h>
#include "debounced_button.h"
#include "button_gesture.h"
#include "usb_protocol.h"
#include "ui_layout.h"
#include "ring_feedback.h"

namespace {
constexpr int kPower = 15, kEncoderA = 4, kEncoderB = 5;
constexpr int kEncoderKey = 0, kUserKey = 6;
constexpr uint16_t kBg = 0x2945, kPanel = 0x39c7, kFg = 0xef5b;
constexpr uint16_t kMuted = 0xa514, kGreen = 0xb5a8, kYellow = 0xf60c, kBlue = 0x8cce, kRed = 0xf2a9;
TFT_eSPI display;
TFT_eSprite ui(&display);
RotaryEncoder encoder(kEncoderA, kEncoderB, RotaryEncoder::LatchMode::TWO03);
DebouncedButton encoderButton, userButton;
ButtonGesture knobGesture, userGesture;
// Vendor CC1101 board: eight addressable pixels on GPIO14, shared power GPIO15.
constexpr int kLedPin = 14, kLedBrightness = 24, kLedDirection = -1;
Adafruit_NeoPixel strip(RingFeedback::kCount, kLedPin, NEO_GRB + NEO_KHZ800);
RingFeedback::Ring ring;
RingFeedback::Frame lastLedFrame{};
bool ledFrameValid = false;
portMUX_TYPE encoderMux = portMUX_INITIALIZER_UNLOCKED;
HostState host;
LineBuffer<1024> incoming;
uint32_t seq = 0, rx = 0;
long position = 0;
bool dirty = true, spriteReady = false;
char activity[40] = "Start embeddeck daemon";

void IRAM_ATTR tickEncoder() {
    portENTER_CRITICAL_ISR(&encoderMux);
    encoder.tick();
    portEXIT_CRITICAL_ISR(&encoderMux);
}

template <size_t Size> void send(const StaticJsonDocument<Size>& doc) {
    // Best-effort input: a blocked/disconnected host must not freeze the UI.
    if (!Serial || Serial.availableForWrite() < int(measureJson(doc) + 1)) return;
    serializeJson(doc, Serial); Serial.write('\n');
}

void hello() {
    StaticJsonDocument<128> doc;
    doc["v"] = 2; doc["type"] = "hello"; doc["firmware"] = "0.3.1";
    send(doc);
}

void status() {
    StaticJsonDocument<384> doc;
    doc["v"] = 2; doc["type"] = "status";
    doc["workspace"] = host.workspace;
    doc["connected"] = host.online(millis()); doc["rx"] = rx;
    doc["mode"] = host.mode; doc["focus_axis"] = host.focusAxis; doc["width_mode"] = host.widthMode;
    send(doc);
}

void sendEncoder(int delta) {
    StaticJsonDocument<128> doc;
    doc["v"] = 2; doc["type"] = "encoder"; doc["seq"] = ++seq; doc["delta"] = delta;
    send(doc);
}

void sendButton(const char* key, ButtonGesture::Event event) {
    StaticJsonDocument<192> doc;
    doc["v"] = 2; doc["type"] = "button"; doc["seq"] = ++seq;
    doc["button"] = key; doc["action"] = event == ButtonGesture::Hold ? "hold" : "click";
    send(doc);
}

void label(const char* text, int x, int y, uint16_t color = kFg, int size = 1) {
    ui.setTextDatum(TL_DATUM); ui.setTextColor(color); ui.setTextSize(size);
    ui.drawString(text, x, y, 1);
}

void updateRing(uint32_t now) {
    using RingFeedback::Mode;
    const bool width = strcmp(host.mode, "width") == 0;
    const Mode mode = width ? Mode::Width : (strcmp(host.mode, "focus") == 0 ? Mode::Focus : Mode::Workspace);
    const bool available = host.online(now) && (!width || (host.windowWidth > 0 && !strcmp(host.layout, "hyprscrolling")));
    const auto frame = ring.render(mode, host.widthPercent, available, now);
    if (ledFrameValid && frame == lastLedFrame) return;
    for (int i = 0; i < RingFeedback::kCount; ++i) {
        const auto c = frame[i]; strip.setPixelColor(i, c.r, c.g, c.b);
    }
    strip.show();
    lastLedFrame = frame; ledFrameValid = true;
}

void render() {
    if (!spriteReady) return;
    const bool online = host.online(millis()), linked = host.linkAlive(millis());
    const bool focus = strcmp(host.mode, "focus") == 0, width = strcmp(host.mode, "width") == 0;
    const bool vertical = strcmp(host.focusAxis, "vertical") == 0, preset = strcmp(host.widthMode, "preset") == 0;
    const uint16_t accent = online ? (focus ? kBlue : (width ? kYellow : kGreen)) : kMuted;
    ui.fillSprite(kBg);
    label("EMBEDDECK", 10, 10, kFg, 2);
    char workspace[12]; snprintf(workspace, sizeof(workspace), "WS%d", online ? host.workspace : 0);
    label(workspace, 126, 15, kMuted);
    const char* modes[] = {"WS", "FOCUS", "WIDTH"};
    const char* ids[] = {"workspace", "focus", "width"};
    for (int i = 0; i < 3; ++i) {
        const bool selected = strcmp(ids[i], host.mode) == 0;
        ui.fillRoundRect(10 + i * 50, 33, 46, 20, 4, selected ? accent : kPanel);
        ui.setTextSize(1); ui.setTextDatum(MC_DATUM); ui.setTextColor(selected ? kBg : kMuted);
        ui.drawString(modes[i], 33 + i * 50, 43, 1);
    }
    ui.fillRoundRect(10, 61, 150, 99, 8, kPanel);
    label(focus ? (vertical ? "FOCUS / VERTICAL" : "FOCUS / HORIZONTAL") :
          (width ? (preset ? "WIDTH / PRESETS" : "WIDTH / 5% STEP") : "WORKSPACE"), 18, 70, accent);
    char value[16];
    if (!online) snprintf(value, sizeof(value), "--");
    else if (focus) snprintf(value, sizeof(value), vertical ? "U/D" : "L/R");
    else if (width && host.windowWidth > 0) snprintf(value, sizeof(value), "~%d%%", host.widthPercent);
    else if (width) snprintf(value, sizeof(value), "--");
    else snprintf(value, sizeof(value), "%02d", host.workspace);
    const int valueSize = strlen(value) > 3 ? 4 : (strlen(value) > 2 ? 6 : 7);
    ui.setTextSize(valueSize); ui.setTextDatum(MC_DATUM); ui.setTextColor(accent);
    ui.drawString(value, 85, 115, 1);
    char detail[25];
    if (!online) snprintf(detail, sizeof(detail), linked ? "BACKEND OFFLINE" : "USB / HOST OFFLINE");
    else if (width) snprintf(detail, sizeof(detail), "%d px / approximate", host.windowWidth);
    else snprintf(detail, sizeof(detail), "%.24s", host.app);
    for (char* p = detail; *p; ++p) if ((unsigned char)*p < 32 || (unsigned char)*p > 126) *p = '?';
    label(detail, 17, 146, kMuted);

    char hint[25];
    if (!linked) snprintf(hint, sizeof(hint), "Start embeddeck daemon");
    else if (host.error[0]) snprintf(hint, sizeof(hint), "%.24s", host.error);
    else if ((focus || width) && strcmp(host.layout, "hyprscrolling")) snprintf(hint, sizeof(hint), "Needs hyprscrolling");
    else snprintf(hint, sizeof(hint), "%.24s", activity);
    for (char* p = hint; *p; ++p) if ((unsigned char)*p < 32 || (unsigned char)*p > 126) *p = '?';
    label(hint, 12, 169, kMuted);
    const int first = WorkspaceUI::firstWorkspace(online ? host.workspace : 1);
    for (int slot = 0; slot < 10; ++slot) {
        const int id = first + slot; const auto cell = WorkspaceUI::workspaceCell(slot);
        const bool active = online && host.workspace == id;
        ui.fillRoundRect(cell.x, cell.y, cell.w, cell.h, 4, active ? accent : kPanel);
        ui.setTextSize(1); ui.setTextDatum(MC_DATUM);
        ui.setTextColor(active ? kBg : (host.occupied(id) && online ? kFg : kMuted));
        char item[12]; snprintf(item, sizeof(item), "%d", id);
        ui.drawString(item, cell.x + cell.w / 2, cell.y + 11, 1);
        if (!active && host.occupied(id) && online) ui.drawFastHLine(cell.x + 8, cell.y + 21, 10, accent);
    }
    ui.drawFastHLine(10, 249, 150, kPanel);
    label("TURN", 12, 257, accent);
    label(focus ? (vertical ? "up / down" : "left / right") : (width ? (preset ? "preset width" : "-5% / +5%") : "prev / next"), 53, 257);
    label("CLICK", 12, 273, accent);
    label(focus ? "switch X / Y" : (width ? "fine / preset" : "last workspace"), 53, 273);
    label("HOLD", 12, 289, accent);
    label(focus ? "focus history" : (width ? "fit active" : "workspace 1"), 53, 289);
    label("USER:mode / hold:WS", 12, 311, linked ? kFg : kMuted);
    ui.pushSprite(0, 0);
}

void receive() {
    for (int budget = 0; budget < 2048 && Serial.available(); ++budget) {
        if (!incoming.push(char(Serial.read()))) continue;
        HostState next = host;
        if (applyState(incoming.data(), next, millis())) {
            const bool changed = !host.linkAlive(millis()) || next.workspace != host.workspace ||
                next.connected != host.connected || strcmp(next.error, host.error) ||
                strcmp(next.mode, host.mode) || strcmp(next.focusAxis, host.focusAxis) || strcmp(next.widthMode, host.widthMode) ||
                strcmp(next.layout, host.layout) || strcmp(next.app, host.app) || next.windowWidth != host.windowWidth ||
                next.widthPercent != host.widthPercent || next.count != host.count ||
                memcmp(next.workspaces, host.workspaces, sizeof(host.workspaces));
            host = next; ++rx;
            if (changed) { dirty = true; status(); }
        } else {
            StaticJsonDocument<256> doc;
            if (!deserializeJson(doc, incoming.data()) && doc["v"] == 2 && doc["type"] == "ack" &&
                doc["ok"].is<bool>() && !doc["ok"].as<bool>()) {
                snprintf(activity, sizeof(activity), "Host action failed"); dirty = true;
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
    // v2 host states can exceed the core's 256-byte default RX queue.
    // Keep whole NDJSON frames and short event bursts while the LCD redraws.
    Serial.setRxBufferSize(4096); Serial.setTxBufferSize(1024);
    Serial.begin(115200);
    attachInterrupt(digitalPinToInterrupt(kEncoderA), tickEncoder, CHANGE);
    attachInterrupt(digitalPinToInterrupt(kEncoderB), tickEncoder, CHANGE);
    display.init(); display.setRotation(WorkspaceUI::kRotation);
    pinMode(TFT_BL, OUTPUT); digitalWrite(TFT_BL, HIGH); display.fillScreen(kBg);
    ui.setColorDepth(16); spriteReady = ui.createSprite(WorkspaceUI::kWidth, WorkspaceUI::kHeight) != nullptr;
    if (!spriteReady) { display.setTextColor(kRed, kBg); display.drawString("UI allocation failed", 10, 30, 2); }
    strip.begin(); strip.setBrightness(kLedBrightness); strip.clear(); strip.show();
    updateRing(millis()); render(); hello();
}

void loop() {
    receive();
    const uint32_t now = millis(); const bool online = host.online(now), linked = host.linkAlive(now);
    static bool wasOnline = false, wasLinked = false;
    if (online != wasOnline || linked != wasLinked) { dirty = true; wasOnline = online; wasLinked = linked; }
    portENTER_CRITICAL(&encoderMux); const long next = encoder.getPosition(); portEXIT_CRITICAL(&encoderMux);
    if (next != position) {
        long delta = next - position; position = next;
        ring.advance(int64_t(delta) * kLedDirection, now);
        snprintf(activity, sizeof(activity), delta > 0 ? "Turn +" : "Turn -");
        if (online) while (delta) {
            const int chunk = delta > 32 ? 32 : (delta < -32 ? -32 : int(delta)); sendEncoder(chunk); delta -= chunk;
        }
        dirty = true;
    }
    const auto knob = readGesture(encoderButton, knobGesture, digitalRead(kEncoderKey) == LOW, linked, now);
    const auto user = readGesture(userButton, userGesture, digitalRead(kUserKey) == LOW, linked, now);
    if (knob != ButtonGesture::None) {
        if (linked) sendButton("knob", knob);
        snprintf(activity, sizeof(activity), knob == ButtonGesture::Hold ? "Knob hold" : "Knob click"); dirty = true;
    }
    if (user != ButtonGesture::None) {
        if (linked) sendButton("user", user);
        snprintf(activity, sizeof(activity), user == ButtonGesture::Hold ? "Home mode / workspace" : "Next mode"); dirty = true;
    }
    static uint32_t lastHello = 0, lastDraw = 0, lastLed = 0;
    if (uint32_t(now - lastLed) >= 25) { updateRing(now); lastLed = now; }
    if (uint32_t(now - lastHello) >= 2000) { hello(); lastHello = now; }
    if (dirty && uint32_t(now - lastDraw) >= 40) { render(); dirty = false; lastDraw = now; }
    delay(1);
}
