# Contributing

このプロジェクトへのコントリビュートありがとうございます。
このドキュメントは、開発参加者向けの最小ガイドです。

## 開発環境

- Go: `1.25.0`
- タスクランナー: `mise`
- Git hooks: `lefthook`

セットアップ:

```bash
mise install
mise run setup
```

Linux ネイティブビルドに必要な依存（Wayland / CGO）:

```bash
mise run deps-linux
```

## よく使うコマンド

- ビルド: `mise run build`
- デバッグビルド: `mise run build-debug`
- 実行: `mise run run`
- Lint: `mise run lint`
- テスト（高速）: `mise run test`
- UI 回帰テスト: `mise run test-ui`
- CI 相当の UI 回帰テスト: `mise run test-ui-ci`
- モジュール整理: `mise run tidy`
- i18n チェック: `mise run check-i18n`
- CI 相当: `mise run ci`

## コーディングルール（抜粋）

- 変更した Go ファイルは `gofmt` を適用してください。
- エラーは `fmt.Errorf("...: %w", err)` で文脈付きにしてください。
- UI 文字列（`internal/ui/`）は i18n ラッパーを必ず使ってください。
  - 推奨: `lang.X(key, fallback)`
  - 新しいキーを追加したら `internal/ui/translations/en.json` と `internal/ui/translations/ja.json` の両方を更新
- 仕様変更時は関連テストを追加・更新してください。

詳細なルールは `AGENTS.md` を参照してください。

## Pull Request の進め方

1. 変更を小さく分け、目的単位でコミットする
2. 事前に `mise run ci` を実行し、UI 変更時は `mise run test-ui-ci` と `mise run check-i18n` も通す
3. PR には「何を変えたか」ではなく「なぜ変えたか」を短く記載する

## 参考ドキュメント

- CI 全体: `docs/build-windows-ci.md`
- 週次 Windows ネイティブ検証: `docs/windows-native-weekly.md`
