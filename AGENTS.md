# AGENTS.md

ISUCON 用の作業テンプレートリポジトリ。競技サーバー（`isucon-1` 等）を SSH 経由で
セットアップ・デプロイし、計測結果をローカルに集めて分析するための Makefile と手順書を持つ。

このファイルは `CLAUDE.md` と同一実体（symlink）。Claude Code / Codex 共通の指示として扱う。

## リポジトリ構成

- `Makefile` — 全操作の入口。setup / deploy / before-bench / after-bench / clear-cache
- `README.md` — 競技開始時の手順書と「秘伝のタレ」（nginx.conf, mysqld.cnf, sysctl 等の設定断片）
- `setup.sh`, `Brewfile` — 競技サーバーへ流し込む shell 環境
- `alp.yml` — alp の出力設定
- `.agents/skills/` — Claude Code / Codex 共有の skill 置き場（`.claude/skills` が symlink）
- 競技中に生成されるもの: `webapp/`, `nginx/`, `mysql/`, `etc/`, `alp/`, `slowquery/`, `profile/`

## 前提

- `Makefile` 冒頭の `SSH_USER` / `ISUCON_USER` / `APP_NAME` / `*_HOST` は競技ごとに書き換える
- サーバー側の作業は基本 `make` 経由。手で ssh するより Makefile にターゲットを足す
- 設定ファイルはサーバーから `make setup-*` でローカルへ落として git 管理し、
  編集後に `make deploy-*` で戻す。サーバー上で直接編集しない

## 計測サイクル

1. `make before-bench` — nginx access.log と mysql-slow.log をローテート
2. ベンチ実行
3. `make after-bench` — ログ / pt-query-digest / pprof をローカルへ回収
4. `alp` と `slowquery/pt-query-digest.log`、`profile/cpu.pprof` を見て次の一手を決める

推測で最適化しない。必ず alp / slow query / pprof の数字を根拠にする。

## 作業上の注意

- 競技中は速度優先。ただし `make deploy-*` は本番相当のサーバーに反映されるので、
  実行前に必ず確認する（reload/restart が走る）
- `clear-cache` はログを削除する破壊的操作。指示されたときだけ実行する
- README の設定断片は動作実績のあるもの。書き換える前に元の値を残す
