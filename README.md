# VRC VRPoker Stats

VRChat の VR Poker ワールド向けに、プレイログを集計して統計を確認できるデスクトップアプリです。

このプロジェクトは、AI を活用した実装をベースにしつつ、人間によるレビューと調整を組み合わせて開発しています。

## ダウンロードして使う（Releases）

1. `https://github.com/AkatukiSora/vrc-vrpoker-ststs/releases` を開きます。
2. 最新リリースの Assets から、お使いの環境に合うファイルをダウンロードします。
   - Linux: `vrpoker-stats-<tag>-linux-wayland.tar.gz`
   - Windows: `vrpoker-stats-<tag>-windows-amd64.zip`
3. 展開して実行します。
   - Linux: `vrpoker-stats`
   - Windows: `vrpoker-stats.exe`

Linux で実行権限がない場合:

```bash
chmod +x vrpoker-stats
./vrpoker-stats
```

## データ保存について

- アプリ起動後、実行したディレクトリに `vrpoker-stats.db` が生成されます。
- 統計データはこの DB に保存されます。
- バックアップしたい場合は `vrpoker-stats.db` をコピーしてください。

## 初回起動

- 起動時に VRChat の `output_log_*.txt` を自動検出して取り込みます。
- 自動検出できない場合は `Settings > Log Source` からログファイルを指定してください。

## 基本操作

- `Overview`: 全体の主要メトリクスとリーク傾向を確認
- `Position Stats`: ポジション別の成績や傾向を確認
- `Hand Range`: 13x13 グリッドでハンドレンジ傾向を確認
- `Hand History`: 記録済みハンドの詳細を確認
- `Settings`: ログファイル設定、表示メトリクス切り替え、About を確認

## トラブルシューティング

- ログが読み込まれない
  - VRChat を起動してプレイ後、最新の `output_log_*.txt` を `Settings > Log Source` で明示指定してください。
- 新しいハンドが反映されない
  - 対象ログファイルが現在書き込まれている VRChat ログか確認してください。
- DB を初期化したい
  - アプリ終了後に `vrpoker-stats.db` を退避または削除して再起動してください。

## コントリビューション

開発参加やビルド手順は `CONTRIBUTING.md` を参照してください。
