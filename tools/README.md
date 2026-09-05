# Tools

`tools/`には、デプロイ、ベンチ計測、RUN管理、分析、可視化を行う再利用可能なツールを置く。
ツール本体は大会固有のアプリケーションへ依存させず、当日の差分は設定ファイル、Taskfile変数、
実行引数、必要な場合だけアプリ側の計測endpointで吸収する。

## 基本方針

- 公式資料と実環境を確認してから設定する。過去大会の値をそのまま採用しない。
- ツール本体を書き換える前に、設定ファイルまたは実行引数で表現できないか確認する。
- collectorを増やす場合は、成果物の宣言・回収・分析側の読み手を同じ変更で揃える。
- 計測負荷も性能へ影響する。baselineでcollectorあり・なしを比較し、間隔や有効化範囲を決める。
- optionalなcollectorやprofilerは、目的と停止・cleanup手順が確認できた場合だけ有効にする。
- deploy、service restart、ログローテート、collector起動はサーバー状態を変更する。対象を確認してから実行する。

このテンプレートの既定構成は、Linux、systemd、cgroup v2、Go、nginx、MySQL、`linux/amd64`である。
異なる構成でも各ツールのcoreを流用できるが、対応するadapterやTaskfileを変更する必要がある。

## 読み方

作業順・service分類・完了判断は[isucon-setup](../.agents/skills/isucon-setup/SKILL.md)を正本とする。本書はツール固有の設定資料であり、下表で対象を選び、詳細節を読む。標準外構成も管理対象に含め、optional機能の詳細は利用時だけ読む。

## ディレクトリ別チェックリスト

| ディレクトリ | 当日の対応 | 確認・変更するもの |
| --- | --- | --- |
| `analysis/` | 成果物・DB・スコア変更時 | [分析](#分析) |
| `analysisctl/` | 原則不要 | `analysis/sources.yaml`を読む汎用core。DuckDB CLIとRUNディレクトリが利用できることを確認する |
| `backlog/` | Objective追加 | coreは変更しない。公式採点仕様からスコア要素、ペナルティ、必須条件をObjectiveへ追加する。採用時点のRUN値はadoption eventへ固定される |
| `bench/` | 一部確認 | `run.sh`が自動・手動ベンチ共通のload windowとfinalizeを担う。nginx on-CPU profilerを使う場合はOS、package manager、kernel用`perf`、probe URLを確認する |
| `browser/` | 原則不要 | ローカルdashboard確認専用。必要な場合だけ`task browser-install`を実行し、競技サーバーへ配布しない |
| `dashboard/` | 条件付き | 標準成果物なら変更不要。独自collector、独自スコア列、追加ロールを表示する場合だけserver APIとweb UIを拡張する |
| `deployctl/` | 毎回確認 | [デプロイ](#デプロイ) |
| `json-metrics-collector/` | 利用時のみ実装 | [アプリ固有adapter](#アプリ固有adapter) |
| `measurectl/` | 毎回確認 | [計測と集計](#計測と集計) |
| `mysql-metrics/` | 接続・互換性確認 | `-dsn`、認証、socket/port、MySQL version、`performance_schema.data_lock_waits`の利用可否を確認する |
| `nginx-backend-report/` | log列確認 | [access log](#access-log) |
| `nginx-oncpu-profiler/` | 利用時のみ環境調整 | 既定では無効。`perf`権限、kernel package、worker数、delay、duration、frequency、出力上限を確認する |
| `proc-metrics/` | service設定 | Linuxの`/proc`、`/sys`、cgroup v2を使う。`-services`へ実際のsystemd unitを渡し、sampling intervalを確認する |
| `topology-screen/` | 利用時に引数調整 | 配置候補を調べる場合だけ、key field・regex、route、hash、分割数、対象bucketを当日のデータモデルへ合わせる |
| `user-transition-metrics/` | adapter設定 | [アプリ固有adapter](#アプリ固有adapter)・[access log](#access-log) |

## 設定項目

### Taskfile

ホスト、役割、IP、アプリ名、DB名、service、port、build、ベンチコマンドを実環境へ合わせる。OS・CPU architectureと、service・設定・ログのパスを確認して値を決める。

### デプロイ

`tools/deployctl/deployments.yaml`では、少なくとも次を確認する。

- アプリのlocal/remote directoryと除外対象
- systemd unitのlocal/remote path
- restart、reload、構文検査、active確認のコマンド
- nginx、DB、アプリのroleとdeploy依存順
- 役割変更時に停止・起動するservice（`check-roles`も同じrole入力を使う）

一般的なGo + nginx + MySQL構成では、ホスト、アプリ名、DB名、service名、directoryは`Taskfile.yml`の
変数から渡せる。複数アプリservice、別DB、container、release symlink方式を使う場合は
`deployments.yaml`を拡張する。

配布の`local`と`remote`には`{host}`を指定できます。例えば`local: 'etc/hosts/{host}/app.conf'`、
`remote: '/home/isucon/app.conf'`でホスト別の設定を配布できます。全対象ホストのパスと転送元ファイルを
転送開始前に検証します。既存の配布先パス制限は引き続き適用されます。

### 計測と集計

`run begin/finalize`がRUN lifecycleを担う。`tools/measurectl/collectors.yaml`では、次を確認する。

- access logとslow logのremote path
- collectorを動かすrole
- `services`と`profile_url`のTaskfile変数、journal権限
- collector binary・実行コマンド、sampling interval、timeout、出力上限
- optional collectorの有効・無効

`tools/measurectl/digesters.yaml`では、次を確認する。

- access logが`alp.yml`および各集計ツールの期待する形式か
- `alp`、`zstdcat`、`pt-query-digest`、`slp`がローカルで利用できるか
- MySQL slow logの利用可否と、performance schemaのqueryが当日のversionで利用できるか
- 成果物を追加・削除したときに、fallback headerと分析側のschemaが一致しているか

標準設定ではproc/service/disk、MySQL status、nginx access log、slow query、app/nginx/kernel journalを収集する。journalは`run.json.load_window`と同じ時間窓で回収できることを確認する。
50msのlock waitと250msのtask stateは既定無効であり、対象環境で負荷を測ったうえで
`tools/measurectl/collectors.yaml`の各`enabled_by_default`を`true`へ変更して採用する。

digesterも`enabled_by_default: false`で既定の実行対象から外せます。省略時は従来どおり有効です。
無効なdigesterの出力は任意成果物となり、未生成でも必須成果物の欠損にはなりません。
`measurectl digest -only <name>`で明示的に実行できます。sourceのログ回収は独立しており、この設定では停止しません。

`MEASURECTL_ROLES`に追加した任意のrole（例: `-role cache=host-a,host-b`）は、
`run.json`の`roles.additional`に保存され、RUN比較では追加・変更・削除を検出します。
意図した差分は`COMPARE_ALLOWED_ROLES=cache`で宣言します。既存の標準roleフィールドと`scores.tsv`形式は維持します。
直接`manifest begin`を使う場合は`-role cache=host-a,host-b`を渡してください。
分析の`manifests.additional_roles`からも追加roleを参照できます。古いRUNの未記録roleは推測で補いません。

GoのCPU、heap、allocs、goroutine profileも既定無効である。アプリがboundedな計測用portで
`net/http/pprof`を公開していること、profile取得時間がベンチ時間内に収まることを確認してから
`task go-profiles-collect`で収集する。公開用traffic portへ無条件にpprofを露出しない。

### access log

標準集計では、nginxのJSON access logに次の列を用意する。

- 共通: `msec`、`method`、`uri`、`status`、`response_time`、`body_bytes`
- upstream分析: `upstream_time`、`upstream_addr`、`upstream_status`、`cache_status`
- ユーザー遷移: Cookie由来の専用識別列。既定名は`session_id`

識別値は個人情報を含めず、成果物へ値やhashを保存しない。ユーザー遷移を使わない場合は、
専用識別列と`user-transitions` digesterを無理に有効化する必要はない。

### 分析

`tools/analysis/sources.yaml`は、RUN成果物とDuckDB schemaの対応表である。標準成果物だけなら変更しない。
成果物名・列・DB種別を変更する場合は、次の宣言と読み手（queryを含む）を一つの変更で揃える。

1. collectorまたはdigesterの出力宣言
2. `run.json`へ記録されるartifact契約
3. `analysis/sources.yaml`のsource
4. 読み込みschemaとsemantic view
5. 必要ならdashboardのAPIと表示

ベンチ出力に独自のスコア内訳やシナリオ結果がある場合は、保存するmarker形式を先に固定してから
`analysis/schema/bench-summary.sql`を拡張する。

### アプリ固有adapter

`tools/user-transition-metrics/routes.json`のCookie列・API prefix・正規化routeを当日のAPIへ合わせる。routeは動的IDを
そのまま残さず、限定された正規表現を上から具体的な順に並べる。Cookieを持たない競技や、識別子を
安全にログへ出せない場合は、この集計を無効にする。

`tools/json-metrics-collector`は、標準では利用しない。既存のHTTP・DB・profile計測では見えない、
スコアに直接関係するboundedなアプリ内部状態がある場合だけ導入する。高cardinalityのラベルや
ユーザーIDを出力しない。利用時はアプリ側にboundedなenable/snapshot/disable endpointを実装し、metric・scope・上限を決めてcollector宣言を追加する。

## 検証コマンドの意味

- `task setup-check`は`task test-tools`、`task artifacts`、`task backlog -- validate`を含む。個別検査と全体検査が重なる場合、通過済みの検査は変更・失敗・未解決の懸念がない限り繰り返さない。
- `task deploy-*-dry`はdeployctlの`-dry-run`でrole・転送元/先・activation順を検証する。Go Taskの`task --dry`は表示のみで、代用できない。
- 採用ゲートは[Backlog workflowのAdoption](backlog/backlog-workflow.md#adoption)に従う。`task pass`が失敗・未判定・スコア不明・不整合controlを拒否することを確認する。
