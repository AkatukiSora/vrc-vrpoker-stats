# 8. Fyne UIテスト戦略

Date: 2026-08-18
日付: 2026-08-18

## Status

accepted

## 背景

AO の Browser パネルは Chromium を操作するため、ネイティブ Fyne
ウィンドウを DOM として検査・操作・撮影することはできない。一方で、UI
には非同期の詳細読み込みと独自レンダラーがあり、UI 変更を継続的に検証する
必要がある。

## 決定

Fyne の in-memory `test` driver を UI 回帰テストの標準とする。テストは実際の
widget interaction と renderer の状態または小さな raster 不変条件を検証し、CI
では `internal/ui` の全テストを実行する。

ネイティブの Windows/Linux visual smoke は、決定的な起動・ready・capture・終了
の test fixture を用意できた時点で追加する。それまでは Xvfb だけを追加しない。

domain/application は Fyne に依存させず、UI は `application.AppService` を介して
依存する。より複雑なタブは必要に応じて presenter/view-model を導入する。Web
frontend への置換はこの決定の範囲外とする。

## 結果

### メリット
- display server を使わず、CI で widget interaction と renderer 回帰を検出できる。
- parser・stats・persistence・application の既存テスト境界を保てる。
- 将来の別クライアントは application の境界から段階的に検討できる。

### トレードオフ
- test driver はネイティブ GPU、OS のフォント、ファイルダイアログを再現しない。
- native screenshot smoke には専用 fixture と OS ごとの実行環境が必要になる。
- web UI は AO Browser で操作できるが、別スタックへの移行コストを正当化する
  利用者価値が必要である。
