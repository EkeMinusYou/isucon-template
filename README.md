# ISUCON template

ISUCONの競技サーバーを、ローカルのリポジトリを正本としてセットアップ・デプロイし、
ベンチ1回ごとのログ・メトリクス・profileを同じ`RUN_ID`へ回収するためのテンプレートです。

## 必要なローカルツール

- [go-task](https://taskfile.dev/)
- Go
- rsync / ssh
- alp
- pt-query-digest
- zstd
- DuckDB
- Node.js（dashboardを使う場合）
- Graphviz（pprof SVGを使う場合）

macOSでは必要に応じてHomebrewで導入してください。競技サーバーには、アプリとcollectorの
Linuxバイナリをローカルでクロスコンパイルして配布します。

## 競技開始時

最初に`Taskfile.yml`冒頭を当日の環境へ合わせます。

```yaml
vars:
  SSH_USER: ubuntu
  ISUCON_USER: isucon
  APP_NAME: app-binary
  APP_DIR: webapp/go
  SERVICE: app-service
  DB_NAME: app-database
  TARGET_OS: linux
  TARGET_ARCH: amd64
  SCHEMA_PATHS: webapp/sql
  # Use SCHEMA_IN_WEBAPP=true instead when initialization assets are confirmed
  # to be included in webapp but do not have a stable standalone path.
  CONFIG_CHECK_COMMAND: '' # Set a contest-specific local validation command.
  SETUP_SECRET_ALLOWLIST: ''
  ALL_HOSTS: isucon-1 isucon-2 isucon-3
  IP:
    map:
      isucon-1: 192.0.2.11
      isucon-2: 192.0.2.12
      isucon-3: 192.0.2.13
  APP_HOSTS: isucon-1
  APP_TRAFFIC_HOSTS: isucon-1
  NGINX_HOSTS: isucon-1
  MYSQL_HOST: isucon-1
  MYSQL_HOSTS: '{{.MYSQL_HOST}}' # Deployment targets; expand only when needed.
```

`~/.ssh/config`に同じホストaliasを設定し、読み取りで実環境と公式資料を確認してから取得します。

```shell
task
task inspect-hosts
task setup
task gen
task setup-check
```

`setup-webapp`の取得対象は`SETUP_WEBAPP_EXCLUDES`で指定したrsync除外ファイルで調整できます。
既定では従来どおり`node_modules/`だけを除外します。ビルド成果物の追加除外、schemaのsymlink、
実サーバーでの設定構文検査は[取得と構文検査の手引き](tools/setup/README.md)を参照してください。

`MYSQL_HOSTS`はMySQL設定の配布・serviceの起動対象、`MYSQL_HOST`は詳細計測先と共通設定の取得元です。
既定は同じ1台です。複数台のときは`MYSQL_HOSTS`を空白区切りで指定し、その中の1台を`MYSQL_HOST`にします。
`db`と標準の`check-network`も`MYSQL_HOST`を使います。アプリのDB接続設定は変更しません。
各アプリが別DBへ接続する構成では、その接続設定と疎通検査を実配置に合わせてください。

取得した`webapp/`、`nginx/`、`mysql/`、`etc/`は最初のbaselineとしてcommitします。
当日マニュアルとAPI仕様は`docs/official/`へ保存してください。
`setup-check`は既定のdocumentation IP、未取得ファイル、schema確認、設定構文検査、必要ツール、
credentialらしいファイルを検出し、build・test・成果物契約・deploy dry-runまで確認します。
必要なcredentialを意図的に管理する場合だけ`SETUP_SECRET_ALLOWLIST`へリポジトリ相対pathを列挙してください。

## 正規デプロイ経路

```shell
task deploy          # build + app配布 + restart
task deploy-nginx    # 設定上書き・nginx -t成功後にreload
task deploy-mysql    # MySQL設定配布 + restart
task deploy-sysctl   # 全ホストへ配布 + sysctl -p
task deploy-all      # 上記を依存順に反映。DB初期化はしない
task apply-roles     # 役割変更後だけenable/disableを収束
task check-roles
task check-network
```

サーバーを変更せず実際の転送先・role・activationを確認する場合は、Go Task自身の`--dry`ではなく
deployctlのdry-runを呼ぶ次のTaskを使います。

```shell
task deploy-app-dry
task deploy-all-dry
```

`tools/deployctl/deployments.yaml`が転送とactivationの差分、`Taskfile.yml`が役割と値を持ちます。
新しいサービスが必要ならdeploymentを追加し、通常の変更に一時的な迂回Taskを増やさないでください。

## ベンチ計測

ユーザーが別端末やポータルからベンチを手動実行する場合:

```shell
task bench-manual
```

ベンチホストから実行できる場合は、`BENCH_COMMAND`を更新して次を使います。

```shell
task bench
```

profile自動収集は既定では無効です。[Go profile導入例](docs/special-sources/go-profiling.md)に沿って
全APP_HOSTSへendpointを導入し、`PROFILES_ENABLED=true`にすると、通常の`bench` / `bench-manual`で
CPU・fgprof・heap・allocs・goroutineを自動収集します。CPU・fgprofの開始を確認してから負荷を開始し、
収集・回収完了後にRUNを確定します。追加のprofile操作は不要です。

分割して操作する場合（profile無効時）:

```shell
task before-bench
# ベンチ実行
task after-bench SCORE=12345
```

実行前に`pwd`、使用するTaskfile、対象ホストを確認してください。collectorの残存が検出されたら、
そのRUN IDとローカルの`raw/current-run-id`を照合します。別checkoutで計測中の可能性があるため、
所有元と終了状態を確認してから対処します。`abort-run`はこの設定が探索するcollectorをまとめて掃除します。
`before-bench`後に中断したRUNを破棄する場合だけ`task abort-run`を使います。通常の回収は必ず`after-bench`です。
`task bench`と`task bench-manual`は、ベンチ失敗や割り込みでも可能な限り`after-bench`を実行し、
失敗RUNをEvidenceとしてfinalizeします。共通の開始・終了・trap処理は`tools/bench/run.sh`、
RUN状態遷移とcollector・digest・manifest処理は`measurectl run begin/finalize`が担当します。

`after-bench`は回収・manifest確定後、そのRUNディレクトリ全体（`.gitignore`対象は除外）と
`runs/scores.tsv`を自動でローカルGitコミットします。失敗RUNも対象です。別RUN、
`raw/`、アプリ・設定変更は含めず、無関係なステージ済み変更も維持します。自動コミットではGit hooksを実行しません。
対象ファイルが既にステージ済みの場合は、そのステージ内容を保護するためコミットを中止してエラーにします。
Gitコミットに失敗しても回収済み成果物は残ります（`git add`後の失敗では対象成果物がステージに残ります）。
RUNは確定済みなので`after-bench`を再実行せず、対象ファイルを確認して手動コミットしてください。pushは行いません。

collector負荷は、同じ構成で通常RUNと次のRUNを取り、スコアとホストメトリクスを比較します。

```shell
task bench-no-collectors
# ポータルベンチの場合
task bench-manual-no-collectors
```

collectorなしRUNは`run.json.collectors_disabled=true`を記録し、周期collectorの成果物を任意扱いにします。
no-collectorsタスクではprofileも無効にしますが、nginx等のログ出力・回収は継続します。
access logやdigesterの必須成果物の欠損は引き続き検査失敗です。異なるcollectorモードのRUNは
通常の採用比較では互換とせず、計測負荷の比較として扱います。古いRUNの省略値は`false`として読みます。

250msのtask-state走査と50msのMySQL lock wait取得は、計測負荷を確認するまで既定では無効です。
利用する場合は`tools/measurectl/collectors.yaml`の`task-state`と`mysql-locks`について
`enabled_by_default: true`へ変更します。設定変更後は通常の`task bench` / `task bench-manual`で収集されます。

collector負荷の比較が終わるまでは`false`を維持し、採用・非採用の判断と実測RUNを設定変更のEvidenceとして残します。

主な成果物:

- `runs/<RUN_ID>/run.json` — source、役割、APPLIED snapshot、score、成果物状態、計測窓
- `alp.txt` / `alp.json` / `alp-by-ingress.tsv`
- `pt-query-digest.log` / `slp.tsv` / `mysql-digest.tsv`
- `<host>-proc-metrics.tsv` / service / disk / task-state（task-stateは高頻度collector明示時）
- `mysql-status.tsv` / `mysql-lock-waits.tsv`（lock waitは高頻度collector明示時）
- `<host>-fgprof.pprof`（profile有効時）
- `<host>-go-cpu.pprof` / heap / allocs / goroutine（profile有効時）
- `raw/access-<host>.log.zst` — ホスト別nginxログ。空ログも圧縮して保存
- `<host>-app-journal.log` / `<host>-nginx-error.log`
- `<host>-kernel.log` / `<host>-oom.log`
- `upstream-breakdown*.tsv`
- `user-transitions.json` — `routes.json`を当日のAPIへ合わせた場合

nginxのJSON access logは、少なくとも`msec`、`method`、`uri`、`status`、`response_time`、`body_bytes`、
`upstream_time`、`upstream_addr`、`upstream_status`、`cache_status`を出してください。ユーザー遷移を使う場合は、個人情報を保存せず、
Cookie由来識別子を専用フィールド（既定`session_id`）に出します。識別値は集計中だけhash化され、成果物には残りません。

`tools/measurectl/collectors.yaml`と`digesters.yaml`は宣言が正本です。成果物を増減したら次を実行します。

```shell
task artifacts
task artifacts-run RUN=runs/<RUN_ID>
```

`task artifacts`は宣言と読み手の整合、`task artifacts-run`は実RUNの必須成果物を検査します。
完全に生成されなかった必須成果物も`run.json`へ`status: missing`として記録されます。
開始時にホスト別の必須ログ・profileを`required_artifacts`へ固定し、形式と計測窓を検査します。
`after-bench`もfinalize・自動ローカルcommit後にこの検査を行い、欠損・不正な内容があれば非0で終了します。
過去のRUNに新しい必須条件を遡及適用しません。

`PROFILE_SECONDS`は初期化・整合性チェック・負荷時間・余裕を含めて設定します。既定の120秒は例です。
heap・allocs・goroutineは`SNAPSHOT_PROFILE_DELAY`後に同時取得します。既定の40秒も競技に合わせて変更します。
自動収集には標準pprofに加え、RUN所有情報を返す開始確認endpointが必要です。詳しくは導入例を参照してください。

```shell
task go-profile-top RUN=runs/<RUN_ID> PROFILE=isucon-1-go-cpu.pprof
```

`scores.tsv`の空欄はスコア不明、`0`は実際の0点です。ベンチ後の採用は`task pass`で行います。
採用条件とFORCEの例外は[Backlog workflow](tools/backlog/backlog-workflow.md#adoption)、
採用記録と`outcomes.tsv`の関係は[Backlog README](tools/backlog/README.md#adoption-records)を参照してください。

## 分析

```shell
task alp
task q-build
task q -- "select run_id, score from runs order by score desc"
task dashboard
```

dashboardでは収集済みのfgprofに加え、GoのCPU・heap・allocs・goroutine profileを
種別・ホスト別に切り替え、関数ランキングとコールグラフで確認できます。コールグラフ表示にはGraphvizが必要です。

分析は最新RUNだけを眺めず、役割・source・APPLIED snapshot・計測窓が比較可能なRUNを選びます。
CPU実仕事、I/O、lock/queue wait、DB query time、HTTP response timeを分け、変更境界が削減できる量を見積もります。

必要に応じて、保存済みaccess logの配置候補を`task topology-screen`で比較できます。
nginxのon-CPU profile、任意JSON endpointのsnapshot、ダッシュボードのブラウザ確認用CLIも
`tools/`に含まれています。導入やサーバー変更を伴うものは、対応する`task --list`のoptional taskを明示的に実行してください。

## Agent workflow

`.agents/skills/`には4段階のskillがあります。

- `isucon-setup` — 初期取得と正規deploy/bench経路の準備
- `isucon-analyze` — Objective、Constraint、Intervention候補の発見
- `isucon-investigate` — INVESTIGATEの独立検証とREADY安全ゲート
- `isucon-worker` — READYの実装、正規deploy、ベンチ後の採否・rollback

```shell
task backlog -- objective list
task backlog -- constraint list
task backlog
task backlog -- validate
```

台帳の正本は`tools/backlog/backlog.sql`、ローカル生成物は`backlog.sqlite3`です。
最初の書き込み操作でSQL dumpが生成されます。

## 再利用資料

- [`docs/special-sources/`](docs/special-sources/README.md) — nginx、MySQL、systemd、sysctlの設定候補
- [`docs/solutions/`](docs/solutions/README.md) — N+1、index、bulk upsert、非同期化、in-memory、静的配信、PGO、UDSなど

これらは自動適用する完成設定ではありません。公式仕様、現行構成、計測値、rollback条件を確認して採用します。
