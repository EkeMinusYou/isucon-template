# AGENTS.md

ISUCON用の作業テンプレートリポジトリ。競技サーバーをSSH経由でセットアップ・デプロイし、
計測結果をローカルへ集約してEvidenceに基づいて改善する。

このファイルは`CLAUDE.md`と同一実体（symlink）。Claude Code / Codex共通の指示として扱う。

## リポジトリ構成

- `Taskfile.yml` — 全操作の入口。冒頭のホスト・役割・アプリ変数が構成の正本
- `README.md` — 競技開始時のセットアップ、デプロイ、計測手順
- `docs/official/` — 当日マニュアル、仕様、API定義などの正本
- `docs/special-sources/` — 複数構成で再利用できる設定断片
- `docs/solutions/` — スキーマや配置に依存する解決策の例
- `docs/reports/` — 分析レポート。`README.md`の命名規則に従い、既存ファイルを上書きしない
- `.agents/skills/` — Claude Code / Codex共有skills
- `tools/measurectl/` — RUNの開始、collector、回収、集計、manifest
- `tools/deployctl/` — 宣言的な転送・activation・deploy plan
- `tools/analysisctl/`、`tools/analysis/` — RUN横断のDuckDB分析
- `tools/backlog/` — Objective・Constraint・Intervention台帳
- `tools/dashboard/` — 計測結果とbacklogのローカル閲覧UI
- `runs/<RUN_ID>/` — 1走行分の集計結果と`run.json`
- `raw/`、`runs/<RUN_ID>/raw/` — 巨大な生ログ。git管理外

## 前提

- 作業前に`docs/official/`の関連資料を確認する。仕様と他資料が矛盾したら公式資料を優先する
- `Taskfile.yml`冒頭の`APP_NAME`、`SERVICE`、`DB_NAME`、`ALL_HOSTS`、`IP`、`*_HOSTS`を競技ごとに更新する
- サーバー上で直接編集せず、`task setup-*`で取得し、ローカル編集後に`task deploy-*`で反映する
- `nginx/conf.d/upstream.conf`は`task gen`の生成物なので手編集しない
- Goアプリとcollectorはローカルで`linux/amd64`へクロスコンパイルする
- deploy対象は採用した実装だけにし、参考実装を正規経路へ混ぜない

## 計測と改善

1. `task before-bench` — RUN採番、APPLIED snapshot、ログローテート、collector起動
2. ベンチ実行
3. `task after-bench` — collector停止、ログ回収、digest、`run.json`確定
4. alp、slow query、fgprof、時系列メトリクスを同じRUNと時間窓で比較する

推測だけで最適化しない。割合だけで律速を決めず、時間・処理量・待ち・capacityを同じ単位と母数で扱う。
欠損成果物は0とみなさず、`run.json`のartifact statusを確認する。

BacklogはObjective・Constraint・Interventionの三層で管理する。計測、ログ、profile、コード、設定、
公式資料はEvidenceでありカード種別ではない。詳細は`.agents/skills/_shared/`と`tools/backlog/README.md`を参照する。

## 作業上の注意

- `task deploy-*`はサーバーのreload/restartを伴う。実行前に対象ホストと影響を確認する
- `task apply-roles`はサービスのenable/disableを伴う
- `task abort-run`は進行中RUNを破棄し、collectorを掃除する
- `task clear-cache`はログやキャッシュを削除する破壊的操作。明示指示時だけ実行する
- 設定断片は候補値である。元の値、採用理由、rollback条件を残す
