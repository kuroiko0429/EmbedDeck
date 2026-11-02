# EmbedDeck

LILYGO T-Embed CC1101 (ESP32-S3) を、本体無改造の左手デバイスにするプロジェクト。
firmware 0.4.0 は USB CDC の NDJSON v3 と Go デーモンで Hyprland / hyprscrolling を操作する。
v1/v2とは互換性がないため、ファームとデーモンを両方更新すること。

## 操作とUI

ユーザーボタン短押しで WS → FOCUS → WIDTH → FLOAT → WS。長押し（600ms）でWSモードへ戻る。
ダイヤル短押しは離した時に発火、長押しは600msで1回だけ発火して短押しを抑止する。

| 入力 | WS | FOCUS | WIDTH | FLOAT |
| --- | --- | --- | --- | --- |
| ダイヤル回転 | ワークスペース前後 | 左右／上下のフォーカス移動 | 列幅 ±5% またはプリセット刻み | 選択した位置・サイズを10論理pxずつ変更 |
| ダイヤル短押し | 直前のWSへ戻る | 左右／上下を切替 | 微調整／プリセットを切替 | 横移動 → 縦移動 → 幅 → 高さを切替 |
| ダイヤル長押し | WS1へ移動 | フォーカス履歴で戻る | 現在列を画面幅に合わせる | フローティング ⇄ タイル |

WSの初期範囲は1〜10、端で停止（循環しない）。モード・軸・刻みはホストが管理し、USB再接続時も同じデーモン内では維持する。
フォーカス／列幅操作にはアクティブな `hyprscrolling` レイアウトとタイル窓が必要。無関係なレイアウトや浮動窓・全画面に誤適用しない。
`fullscreen_on_one_column=true` では1列だけのWSは全幅になるため、幅変更を見るには2列以上にする。
幅の%表示は窓の実ピクセル幅から計算した概算（`~`付き）で、プラグイン内部の厳密な列幅ではない。

FLOATはHyprland標準APIを使うため、hyprscrolling以外のレイアウトでも利用できる。
浮動窓だけを移動・リサイズし、タイル状態では `TILED / Hold knob to float` と案内する。全画面・非表示・窓なしでは拒否する。
現在モニターの予約領域・拡大率・回転を考慮した作業領域内に制限し、最小サイズは原則160×100論理px（作業領域が小さい場合はそちらを優先）。アプリ側のサイズ制約で変化が止まることもある。
液晶は選択対象の実測値とX/Y/W/Hを表示。負の座標にも対応する。WSへ戻っても浮動状態は勝手に解除しない。

液晶はダイヤルを上にした170×320の縦向き（rotation 0）。大きな現在番号、5列×2段のワークスペース枠、操作ガイド・接続状態を Gruvbox 配色で表示。
既存ワークスペースには下線、現在位置にはハイライトを付ける。
キーボード等で切り替えても液晶を同期する。ホスト状態が3.5秒途切れると OFFLINE になり、オフライン操作は後から再生しない。

これは HID キーボードではなくデーモン経由の操作。現在のバックエンドは Hyprland 0.57 の Lua dispatcher のみ。
他の Linux DE・Windows・macOS・Android 対応、BLE/HID、キー割り当て編集は未実装。

## LEDリング

GPIO14の8灯をAdafruit NeoPixel 1.12.3で制御。明るさは24/255に抑える。

- WS: 緑の点が回転量に追従し、短い残像を付ける
- FOCUS: 同じ回転追従を青で表示
- WIDTH: ホストから受け取った概算幅を黄色のバーで表示（端のLEDは部分点灯）
- FLOAT: 編集可能な浮動窓では紫の回転追従。タイル状態・対象なしは弱いアンバー
- ホスト未接続／幅情報なし: 弱いアンバー1灯

校正は保留。今回はエンコーダーの1デコードステップで隣のLEDへ進む相対表示で、8ステップで光が一周する。物理的なノブ1周とは一致するとは限らず、絶対角度センサーでもない。
`kLedDirection=-1`で実機のLED配列と回転方向を合わせている（ユーザー確認済み）。位置は再起動時に初期化される。
更新上限は25ms間隔、色が変わるときだけRMT経由で送信する。LEDの位置はローカル、モード・浮動状態・幅はホストから確認した値を使う。

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
Linuxでは非ブロッキングTTY＋Goのポーラー／書き込み期限を使用し、詰まった書き込みを約1秒で解除する。単にブロッキングwrite中のfdを別goroutineから閉じる方式には依存しない。raw115200 8N1・DTR/RTS=false・排他アクセスを維持する（期限／Close解除の保証はLinux限定、ハードリアルタイムではない）。
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
- TFT_eSPI 2.5.43 / RotaryEncoder 1.5.3 / ArduinoJson 6.21.5 / Adafruit NeoPixel 1.12.3
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
別プロセスとして起動するため、mode/axis/width_mode/float_actionは初期値であり、稼働中デーモンのモード取得APIではない。実機が適用したモードはjournalの `device state` で確認する。

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

0.4 FLOAT機能:

- `check_floating.py`: 専用の一時窓でHyprland標準の浮動化・X/Y移動・幅/高さ変更・タイル復帰を確認
- `check_float_daemon.py`: PTY入力から実Go・実IPCまで、4操作の正逆、画面端、最小サイズ、全画面/タイルガード、浮動化/タイル復帰とWSモードへの脱出を確認（物理USB試験とは別）
- ネイティブのリサイズが窓を中央基準で移動させる挙動を実測。実寸読戻し後に左上アンカーを補正し、左端/上端でも画面外へ出ないことを回帰テスト・実IPCで確認
- 操作対象の確認と現在フォーカスの表示を分離。途中で別の窓へフォーカスが変わっても、確認済み操作を失敗扱いしないことを私設IPCの回帰テストで確認。ready=true時のゼロ寸法も両側で拒否
- v3/0.4.0を書き込み、実USBから浮動状態・座標・寸法を読戻し。4モード描画中の連続受信16/16を確認
- ユーザー実機確認: 浮動化、X/Y移動、幅/高さ、タイル戻し、紫LEDと液晶の追従すべて正常。実操作ログと、タイル窓への回転が拒否されることも照合
- C++テストとASan/UBSan、Go race/vet、ファームビルドを通過。最大状態フレーム、負の座標、4タブと表示幅も検証
- 検証用の窓を片付け、一時ユーザーサービスを復帰。以前の0.3.1アプリ領域とGo実行ファイルは非公開backups/に退避

0.3.1 LED追加:

- リングモデルの回転・逆回転・整数/時刻周回・残像・幅バーをネイティブテスト（ASan/UBSanも通過）
- firmware 0.3.1の実機書き込み、USB往復、LED更新中の状態連続受信16/16を確認
- ユーザー実機確認: 方向反転後、緑/青/黄の色・回転追従・幅メーター・従来操作すべて正常
- 物理的な1周の角度校正は未実施。相対1ステップ表示として利用

0.3モード機能:

- `check_hyprscrolling.py`: 一時窓2枚で左右／上下／履歴、列幅5%・プリセット・全幅を実IPCで確認し、窓を閉じて元へ復帰
- `check_daemon_modes.py`: PTYから実Goセッション／コントローラ／実IPCへの統合。PTYにはmodem ioctlがないため、テスト専用の `TestLivePTYBridge` でポートopenだけを置換（物理USBの検証ではない）。通常のGoテストでは明示opt-inなしに起動しない
- その実測例: 幅933→1027px、プリセット1247px、fit active 1878px。空WSでのタイル窓ガードも確認
- 実USB: firmware 0.3.0 / v2 hello とモード入りstatusを読み戻し
- 液晶描画中のUSB連続状態受信: 16送信／16受理。HWCDCの256-byte既定キュー不足に対しRX4096／TX1024を明示設定
- C++テスト: 600ms長押し、短押しとの排他、時刻周回、オフライン押下の復帰時抑止（20msデバウンス境界含む）、過大WS番号の拒否
- Linux通信の私設PTYテスト: 読まれない相手へのwrite期限、詰まったread/writeのClose割込み、セッションcancel、watchdog、排他解除・再openを検証。修正版の実機USB往復も確認
- ユーザー実機確認: FOCUS／WIDTHの操作、モード切替、短押し／長押し、UI追従すべて正常。journalに各モードの実操作成功を記録

基礎通信・以前の確認:

- firmware ビルド・実機書き込み・書き込みハッシュ照合
- C++ ホストテスト: デバウンス、状態 JSON の型検証、heartbeat 期限、oversize フレーム復帰
- Go race test・vet・ビルド。偽 UNIX socket で IPC、dispatcher、履歴、キャンセル、エラー処理をテスト
- 実 IPC の状態取得、Lua dispatcher での切り替え、読み戻し、元の状態への復帰
- 実機 hello -> 実 Hyprland snapshot を USB 送信 -> 実機 status の workspace/RX 読み戻し
- 実ダイヤル -> Go -> Hyprland の操作成功ログ
- 外部 IPC での変更 -> Go -> 実機 status の往復同期と元の状態への復帰
- ユーザー実機確認: ダイヤルを上にした縦UIの向き・配置・操作が正常
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
