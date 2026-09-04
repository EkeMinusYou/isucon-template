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
```

`~/.ssh/config`に同じホストaliasを設定し、読み取りで実環境と公式資料を確認してから取得します。

```shell
task
task inspect-hosts
task setup
task gen
task setup-check
```

取得した`webapp/`、`nginx/`、`mysql/`、`etc/`は最初のbaselineとしてcommitします。
当日マニュアルとAPI仕様は`docs/official/`へ保存してください。
`setup-check`は既定のdocumentation IP、未取得ファイル、schema確認、設定構文検査、必要ツール、
credentialらしいファイルを検出し、build・test・成果物契約・deploy dry-runまで確認します。
必要なcredentialを意図的に管理する場合だけ`SETUP_SECRET_ALLOWLIST`へリポジトリ相対pathを列挙してください。

## 正規デプロイ経路

```shell
task deploy          # build + app配布 + restart
task deploy-nginx    # nginx -t後にreload
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

ポータルから手動実行する場合:

```shell
task bench-manual
```

ベンチホストから実行できる場合は、`BENCH_COMMAND`を更新して次を使います。

```shell
task bench
```

分割して操作する場合:

```shell
task before-bench
# ベンチ実行
task fgprof-collect   # アプリがfgprof endpointを公開している場合
task after-bench SCORE=12345
```

`before-bench`後に中断した場合だけ`task abort-run`を使います。通常の回収は必ず`after-bench`です。
`task bench`と`task bench-manual`は、ベンチ失敗や割り込みでも可能な限り`after-bench`を実行し、
失敗RUNをEvidenceとしてfinalizeします。共通の開始・終了・trap処理は`tools/bench/run.sh`、
RUN状態遷移とcollector・digest・manifest処理は`measurectl run begin/finalize`が担当します。

collector負荷は、同じ構成で通常RUNと次のRUNを取り、スコアとホストメトリクスを比較します。

```shell
task bench-no-collectors
# ポータルベンチの場合
task bench-manual-no-collectors
```

no-collector RUNでproc/MySQL collector成果物が`missing`になるのは意図どおりです。

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
- `<host>-fgprof.pprof`
- `<host>-go-cpu.pprof` / heap / allocs / goroutine（明示収集時）
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

Goアプリが標準`net/http/pprof` endpointを計測用portで公開している場合、進行中RUNへprofileを収集できます。
CPU profileは`PROFILE_DELAY`後から`PROFILE_SECONDS`秒、heap・allocs・goroutineは
`SNAPSHOT_PROFILE_DELAY`後に同時取得します。公開先は`PPROF_BASE_URL`を当日の構成へ合わせてください。

```shell
task go-profiles-collect
task go-profile-top RUN=runs/<RUN_ID> PROFILE=isucon-1-go-cpu.pprof
```

`scores.tsv`の空欄はスコア不明、`0`は実際の0点です。採否はTSVの直前行ではなく`run.json`を正本とし、
`task pass`は`passed=true`かつスコア既知の最新RUNだけを受け付けます。`COMPARE_RUN`がある場合は、
finalize後も`comparison.status=compatible`であることを要求し、そのcontrol RUNとの差分を記録します。
例外的に採用する場合は`task pass FORCE=true`を使います。forceでもfinalized状態、APPLIED snapshot、
カード定義の一致は必須であり、強制採用であることはカードのHistoryへ記録されます。
採用時点のscore、passed、control、delta、manifest hashはBacklog SQLiteのadoption eventとして
カード昇格と同じtransactionに保存され、`outcomes.tsv`はそこから再生成されます。

## 分析

```shell
task alp
task q-build
task q -- "select run_id, score from runs order by score desc"
task dashboard
```

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
