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
  SERVICE: app-service
  DB_NAME: app-database
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
task setup
task gen
task build
task test-tools
```

取得した`webapp/`、`nginx/`、`mysql/`、`etc/`は最初のbaselineとしてcommitします。
当日マニュアルとAPI仕様は`docs/official/`へ保存してください。

## 正規デプロイ経路

```shell
task deploy          # build + app配布 + restart
task deploy-nginx    # nginx -t後にreload
task deploy-mysql    # MySQL設定配布 + restart
task deploy-sysctl   # 全ホストへ配布 + sysctl -p
task deploy-all      # 上記を依存順に反映。DB初期化はしない
task apply-roles     # 役割変更後だけenable/disableを収束
task check-network
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

主な成果物:

- `runs/<RUN_ID>/run.json` — source、役割、APPLIED snapshot、score、成果物状態、計測窓
- `alp.txt` / `alp.json` / `alp-by-ingress.tsv`
- `pt-query-digest.log` / `slp.tsv` / `mysql-digest.tsv`
- `<host>-proc-metrics.tsv` / service / disk / task-state
- `mysql-status.tsv` / `mysql-lock-waits.tsv`
- `<host>-fgprof.pprof`
- `upstream-breakdown*.tsv`
- `user-transitions.json` — `routes.json`を当日のAPIへ合わせた場合

nginxのJSON access logは、少なくとも`msec`、`method`、`uri`、`status`、`response_time`、`body_bytes`、
`upstream_time`、`upstream_addr`、`upstream_status`、`cache_status`を出してください。ユーザー遷移を使う場合は、個人情報を保存せず、
Cookie由来識別子を専用フィールド（既定`session_id`）に出します。識別値は集計中だけhash化され、成果物には残りません。

`tools/measurectl/collectors.yaml`と`digesters.yaml`は宣言が正本です。成果物を増減したら次を実行します。

```shell
task artifacts
```

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
