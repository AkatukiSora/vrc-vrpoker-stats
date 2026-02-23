# VRC VRPoker Stats

<img src="assets/logo.png" alt="VRPoker Stats" height="70"/>

VRChat の VR Poker ワールド向けに、プレイログを自動解析して統計を確認できるデスクトップアプリです。

[![Go 1.25](https://img.shields.io/badge/Go-1.25-00ADD8?logo=go)](https://go.dev/)
[![Platform](https://img.shields.io/badge/platform-Linux%20%7C%20Windows-informational)](#動作環境)
[![License](https://img.shields.io/github/license/AkatukiSora/vrc-vrpoker-ststs)](LICENSE)

> このプロジェクトは、AI を活用した実装をベースにしつつ、人間によるレビューと調整を組み合わせて開発しています。

---

## 機能

| タブ | 内容 |
|---|---|
| **Overview** | VPIP・PFR・bb/100 などの主要指標をカード表示。改善すべきリーク（傾向）を自動検出してアドバイス表示 |
| **Position Stats** | BTN・CO・MP・UTG・SB・BB 各ポジション別の成績・統計テーブル |
| **Hand Range** | 13×13 ハンドレンジグリッド。各セルをクリックするとコンボ別アクション頻度を確認可能 |
| **Hand History** | プレイしたハンドの一覧と詳細（コミュニティカード・ストリート別アクション・結果）。ハンドカテゴリや期間でフィルタ可能 |
| **Settings** | ログファイルパス設定、表示メトリクスのカスタマイズ、データベースリセット |

### 計測できる主なメトリクス

- **プリフロップ**: VPIP, PFR, 3Bet, Fold to 3Bet, Steal, Fold to Steal
- **ポストフロップ**: CBet（Flop/Turn）, Fold to CBet, WTSD, W$SD
- **結果**: bb/100, 総利益/損失
- いずれも `n=` （サンプル数）を併記し、信頼度が低い値は参考値として明示

---

## 動作環境

| OS | 対応状況 |
|---|---|
| Linux (Wayland / X11) | 動作確認済み（Steam Proton 経由の VRChat も対応）|
| Windows | 動作確認済み |
| macOS | 部分対応（ビルド可能だが継続的な確認は未実施）|

---

## ダウンロードして使う

1. [Releases](https://github.com/AkatukiSora/vrc-vrpoker-ststs/releases) を開きます。
2. 最新リリースの Assets から、お使いの環境に合うファイルをダウンロードします。
   - Linux: `vrpoker-stats-<tag>-linux-wayland.tar.gz`
   - Windows: `vrpoker-stats-<tag>-windows-amd64.zip`
3. 展開して実行します。
   - Linux: `./vrpoker-stats`
   - Windows: `vrpoker-stats.exe`

Linux で実行権限がない場合:

```bash
chmod +x vrpoker-stats
./vrpoker-stats
```

---

## 初回起動

起動すると VRChat の `output_log_*.txt` を自動検出してインポートします。

- **自動検出できない場合**: `Settings > Log Source` からログファイルを手動で指定してください。
- ログ取り込み後、VRChat でプレイを続けるとリアルタイムで統計に反映されます。

### ログファイルの場所

| OS | パス |
|---|---|
| Linux (Steam Proton) | `~/.steam/steam/steamapps/compatdata/<AppID>/pfx/users/steamuser/AppData/LocalLow/VRChat/VRChat/` |
| Windows | `%APPDATA%\..\LocalLow\VRChat\VRChat\` |

---

## データ保存について

- 統計データは OS のユーザーデータディレクトリ内の `vrpoker-stats.db`（SQLite）に保存されます。
- バックアップしたい場合はこのファイルをコピーしてください。
- DB を初期化したい場合は `Settings > Data Management > Reset Database` から行えます。

---

## トラブルシューティング

**ログが読み込まれない**
- VRChat を起動してプレイ後、最新の `output_log_*.txt` を `Settings > Log Source` で明示指定してください。

**新しいハンドが反映されない**
- 対象ログファイルが現在書き込まれている VRChat ログか確認してください。

**統計値に `参考値` と表示される**
- サンプル数が少ない場合に表示されます。該当メトリクスの信頼水準に達するまでプレイを続けると消えます。

---

## コントリビューション

開発参加やビルド手順は [`CONTRIBUTING.md`](CONTRIBUTING.md) を参照してください。
