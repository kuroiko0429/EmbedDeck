# EmbedDeck

LILYGO T-Embed CC1101 (ESP32-S3) を、本体無改造の左手デバイスにするプロジェクト。
firmware 0.2.0 は USB CDC の NDJSON と Go デーモンで Hyprland を操作する。

## 操作とUI

- ダイヤル正方向: 次の番号のワークスペース
- ダイヤル逆方向: 前の番号のワークスペース
- ダイヤル押し込み: 直前に訪れたワークスペースへ戻る（往復可能）
- ユーザーボタン: ワークスペース1
- 初期範囲: 1〜10、端で停止（循環しない）

液晶は Gruvbox 配色で、現在番号・10個のワークスペース枠・操作ガイド・接続状態を表示。
既存ワークスペースには下線、現在位置にはハイライトを付ける。
キーボード等で切り替えても液晶を同期する。ホスト状態が3.5秒途切れると OFFLINE になり、オフライン操作は後から再生しない。

これは HID キーボードではなくデーモン経由の操作。現在のバックエンドは Hyprland 0.57 の Lua dispatcher のみ。
他の Linux DE・Windows・macOS・Android 対応、BLE/HID、キー割り当て編集は未実装。

## 起動・停止

このセッションでは `embeddeck.service` を systemd-run の一時ユーザーサービスとして起動済み。
`graphical-session.target` に PartOf で紐付けている。永続ユニットの導入・自動起動の有効化はしていない。
Hyprland の設定・キーバインドは変更していない。

```sh
systemctl --user status embeddeck.service
journalctl --user -u embeddeck.service -f
systemctl --user stop embeddeck.service
```

手動起動（サービスを停止してから）:

```sh
cd /home/kuroiko/Documents/EmbedDeck
./daemon/embeddeck --max-workspace 10
```

別セッションで一時サービスを起動:

```sh
systemd-run --user --unit=embeddeck --collect \
  --property=PartOf=graphical-session.target \
  --property=NoNewPrivileges=yes \
  --setenv=XDG_RUNTIME_DIR="$XDG_RUNTIME_DIR" \
  /home/kuroiko/Documents/EmbedDeck/daemon/embeddeck --max-workspace 10
```

USB は既定でこの実機の by-id パスを使用。別ボードは `--port /dev/serial/by-id/...` を指定。
デーモンとシリアルモニターを同時に開かないこと。ファーム書き込み前にもデーモンを止める。
USB 切断時は1秒間隔で再接続を試行する。
永続化用の参考は `tools/embeddeck.service.example`（未導入）。

## 構成

```text
T-Embed encoder / buttons
  -> USB CDC NDJSON -> Go daemon -> Hyprland .socket.sock -> workspace change
Hyprland .socket2.sock events / heartbeat resync
  -> Go daemon -> USB CDC state JSON -> T-Embed LCD
```

`PROTOCOL.md` にメッセージ・上限・エラー処理を記載。
シリアル経由では固定の操作名と数値だけを受け付け、任意のコマンド実行はしない。
切り替え後の Hyprland 状態を読み戻し、デバイスの受信状態もログに記録する。

`HYPRLAND_INSTANCE_SIGNATURE` が未設定の場合、実際に応答する単一の IPC ソケットを検出する。
複数インスタンスが動いている場合は対象の署名を環境変数で明示する。

## 開発環境とテスト

- PlatformIO Core 6.2.0（uv tool、Python 3.12）
- espressif32 6.13.0 / Arduino-ESP32 2.0.17
- TFT_eSPI 2.5.43 / RotaryEncoder 1.5.3 / ArduinoJson 6.21.5
- Go（依存は daemon/go.mod と go.sum に固定）
- 実機: ESP32-S3 rev 0.2、Flash 16MB、内蔵 PSRAM 8MB（esptool 識別）

```sh
uv tool install --python 3.12 platformio
pio run
bash scripts/test_firmware.sh
cd daemon
go test -race ./...
go vet ./...
go build -o embeddeck .
./embeddeck --probe
```

`--probe` は実 Hyprland 状態の読み取り専用確認。シリアルは開かず、ワークスペース操作もしない。

ファーム書き込み（リポジトリ直下で）:

```sh
systemctl --user stop embeddeck.service
pio run -t upload --upload-port /dev/serial/by-id/usb-Espressif_USB_JTAG_serial_debug_unit_30:ED:A0:34:56:4C-if00
```

PlatformIO 環境名は初期ハードウェア確認時の `hardware-test` を引き継いでいるが、中身は現在の bridge ファーム。
基板名は汎用 DevKitC-1-N8。platformio.ini で Flash 16MB、qio_opi、PSRAM を明示的に上書きする。
TFT_eSPI の TOUCH_CS 未定義警告は非タッチ液晶のため想定内。
補完用データベースは `pio run -t compiledb`。

実 USB 往復テスト（デーモン停止中に実行）:

```sh
~/.local/share/uv/tools/platformio/bin/python scripts/usb_roundtrip.py
```

## 検証結果

- firmware ビルド・実機書き込み・書き込みハッシュ照合
- C++ ホストテスト: デバウンス、状態 JSON の型検証、heartbeat 期限、oversize フレーム復帰
- Go race test・vet・ビルド。偽 UNIX socket で IPC、dispatcher、履歴、キャンセル、エラー処理をテスト
- 実 IPC の状態取得、Lua dispatcher での切り替え、読み戻し、元の状態への復帰
- 実機 hello -> 実 Hyprland snapshot を USB 送信 -> 実機 status の workspace/RX 読み戻し
- 実ダイヤル -> Go -> Hyprland の操作成功ログ
- 外部 IPC での変更 -> Go -> 実機 status の往復同期と元の状態への復帰
- ユーザー実機確認: UI、左右回転、両ボタン、キーボードからの同期、USB 抜き差し後の復帰すべて正常

未検証・未対応: 他 OS/DE、BLE/HID、電池駆動の長時間安定性、ログアウト/再ログイン後の自動起動。

## ハードウェア

| 機能 | GPIO |
| --- | --- |
| 電源維持 | 15 |
| LCD BL / CS / DC | 21 / 41 / 16 |
| SPI MOSI / MISO / SCLK | 9 / 10 / 11 |
| エンコーダー A / B / 押し込み | 4 / 5 / 0 |
| ユーザーボタン | 6 |
| CC1101 / SD / 拡張 nRF CS | 12 / 13 / 44 |

CC1101 版の公式設定に合わせ HSPI を使い、共用 SPI の他デバイスを deselect してから電源・LCD を初期化。
Wi-Fi/BLE/CC1101/NFC/SD/IR 送信は初期化しない。GPIO0 を押したままリセットしないこと。

Linux の tools/70-embeddeck.rules は /etc/udev/rules.d/ に導入済み。
実機の USB シリアル番号に限定して uaccess を付与し、内蔵 LTE モデムは変更しない。

一次資料:

- https://github.com/Xinyuan-LilyGO/T-Embed-CC1101 （確認 commit c36826e0f91311cb591c4545fccf91e254fc55dc）
- examples/utilities.h、examples/encode_test/encode_test.ino、examples/display_test/display_test.ino、lib/TFT_eSPI/User_Setups/Setup214_LilyGo_T_Embed_PN532.h
- https://wiki.hypr.land/IPC/
- 実機の hyprctl version/help と IPC 応答で Lua dispatcher を照合

## バックアップ

初期 Bruce の Flash 全体 16,777,216 bytes を保存し、書き換え前の実機との digest 一致を検証済み。

- backups/bruce-before-embeddeck.bin
- SHA-256: 1eb1dccc7e57f38c14626587c91d1e375b7ad1675f761f9b22f9a3edb1c58163
- 初期確認用0.1の成果物: backups/hardware-test-v0.1/
- USB往復結果: backups/usb-roundtrip.json

Flash バックアップには設定情報等が含まれ得るため公開しない。
バックアップ・ビルド成果物・ローカル検証ログは .gitignore 対象。
書き込みは全 Flash 消去ではなく、以前のデータ領域を完全消去したわけではない。
Bruce 復元は未実施。必要なときだけ、デーモンを止めてから実行（現在の内容を上書き）:

```sh
~/.local/share/uv/tools/platformio/bin/python ~/.platformio/packages/tool-esptoolpy/esptool.py \
  --chip esp32s3 --port /dev/ttyACM0 --baud 921600 \
  write_flash 0 backups/bruce-before-embeddeck.bin
```
