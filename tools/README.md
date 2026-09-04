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

## 競技開始時の設定順

1. `docs/official/`から、変更可能範囲、ホスト構成、採点条件、再起動・初期化条件を確認する。
2. 読み取り専用の確認で、OS、CPU architecture、service名、設定パス、ログパス、port、private IPを記録する。
3. `Taskfile.yml`のアプリ、ホスト、役割、IP、port、build、ベンチコマンドを実環境へ合わせる。
4. 下表に従い、必須adapterを確認する。使わないoptional機能は無理に設定しない。
5. nginx access logのJSON列と、MySQL slow log・performance schemaの利用可否を確認する。
6. ローカルのbuild、test、成果物契約、deploy dry-runを通す。
7. 計測経路を有効にしたbaselineで、collectorの欠損と計測オーバーヘッドを確認する。

## ディレクトリ別チェックリスト

| ディレクトリ | 当日の対応 | 確認・変更するもの |
| --- | --- | --- |
| `analysis/` | 条件付き | 標準RUN成果物なら変更不要。成果物名・列・DB種別・スコア内訳を増減する場合は`sources.yaml`、schema、queryを同時に更新する |
| `analysisctl/` | 原則不要 | `analysis/sources.yaml`を読む汎用core。DuckDB CLIとRUNディレクトリが利用できることを確認する |
| `backlog/` | Objective追加 | coreは変更しない。公式採点仕様からスコア要素、ペナルティ、必須条件をObjectiveへ追加する |
| `bench/` | 一部確認 | `active-run-id.sh`はそのまま使う。nginx on-CPU profilerを使う場合はOS、package manager、kernel用`perf`、probe URLを確認する |
| `browser/` | 原則不要 | ローカルdashboard確認専用。必要な場合だけ`task browser-install`を実行し、競技サーバーへ配布しない |
| `dashboard/` | 条件付き | 標準成果物なら変更不要。独自collector、独自スコア列、追加ロールを表示する場合だけserver APIとweb UIを拡張する |
| `deployctl/` | 毎回確認 | `deployments.yaml`のlocal/remote path、systemd unit、activation、role、依存順を実構成へ合わせる。coreは変更しない |
| `json-metrics-collector/` | 利用時のみ実装 | 既定では無効。アプリ側にboundedなenable/snapshot/disable endpointを実装し、metric、scope、上限を決めてcollector宣言を追加する |
| `measurectl/` | 毎回確認 | `collectors.yaml`と`digesters.yaml`のrole、service、ログパス、profile endpoint、実行コマンド、出力を確認する |
| `mysql-metrics/` | 接続・互換性確認 | `-dsn`、認証、socket/port、MySQL version、`performance_schema.data_lock_waits`の利用可否を確認する |
| `nginx-backend-report/` | log列確認 | nginx JSON logに`status`、`response_time`、`upstream_time`、`upstream_addr`、`upstream_status`、`cache_status`を出す |
| `nginx-oncpu-profiler/` | 利用時のみ環境調整 | 既定では無効。`perf`権限、kernel package、worker数、delay、duration、frequency、出力上限を確認する |
| `proc-metrics/` | service設定 | Linuxの`/proc`、`/sys`、cgroup v2を使う。`-services`へ実際のsystemd unitを渡し、sampling intervalを確認する |
| `topology-screen/` | 利用時に引数調整 | 配置候補を調べる場合だけ、key field・regex、route、hash、分割数、対象bucketを当日のデータモデルへ合わせる |
| `user-transition-metrics/` | adapter設定 | `routes.json`のCookie列、API prefix、正規化routeを当日のAPIへ合わせ、必要なaccess log列が出ていることを確認する |

## 必須adapter

### デプロイ

`tools/deployctl/deployments.yaml`では、少なくとも次を確認する。

- アプリのlocal/remote directoryと除外対象
- systemd unitのlocal/remote path
- restart、reload、構文検査、active確認のコマンド
- nginx、DB、アプリのroleとdeploy依存順
- 役割変更時に停止・起動するservice

一般的なGo + nginx + MySQL構成では、ホスト、アプリ名、service名、directoryは`Taskfile.yml`の
変数から渡せる。複数アプリservice、別DB、container、release symlink方式を使う場合は
`deployments.yaml`を拡張する。

### 計測と集計

`tools/measurectl/collectors.yaml`では、次を確認する。

- access logとslow logのremote path
- collectorを動かすrole
- `services`と`profile_url`のTaskfile変数
- collector binary、sampling interval、timeout、出力上限
- optional collectorの有効・無効

`tools/measurectl/digesters.yaml`では、次を確認する。

- access logが`alp.yml`および各集計ツールの期待する形式か
- `alp`、`zstdcat`、`pt-query-digest`、`slp`がローカルで利用できるか
- MySQL performance schemaのqueryが当日のversionで利用できるか
- 成果物を追加・削除したときに、fallback headerと分析側のschemaが一致しているか

標準設定ではproc/service/disk、MySQL status/lock、nginx access log、slow queryを収集する。
50msのlock wait、250msのtask stateなど短い間隔は、対象環境で負荷を測ってから採用する。

### access log

標準集計では、nginxのJSON access logに次の列を用意する。

- 共通: `msec`、`method`、`uri`、`status`、`response_time`、`body_bytes`
- upstream分析: `upstream_time`、`upstream_addr`、`upstream_status`、`cache_status`
- ユーザー遷移: Cookie由来の専用識別列。既定名は`session_id`

識別値は個人情報を含めず、成果物へ値やhashを保存しない。ユーザー遷移を使わない場合は、
専用識別列と`user-transitions` digesterを無理に有効化する必要はない。

### 分析

`tools/analysis/sources.yaml`は、RUN成果物とDuckDB schemaの対応表である。標準成果物だけなら変更しない。
独自成果物を追加する場合は、次を一つの変更として扱う。

1. collectorまたはdigesterの出力宣言
2. `run.json`へ記録されるartifact契約
3. `analysis/sources.yaml`のsource
4. 読み込みschemaとsemantic view
5. 必要ならdashboardのAPIと表示

ベンチ出力に独自のスコア内訳やシナリオ結果がある場合は、保存するmarker形式を先に固定してから
`analysis/schema/bench-summary.sql`を拡張する。

### アプリ固有adapter

`tools/user-transition-metrics/routes.json`は、当日のAPIへ差し替えるadapterである。routeは動的IDを
そのまま残さず、限定された正規表現を上から具体的な順に並べる。Cookieを持たない競技や、識別子を
安全にログへ出せない場合は、この集計を無効にする。

`tools/json-metrics-collector`は、標準では利用しない。既存のHTTP・DB・profile計測では見えない、
スコアに直接関係するboundedなアプリ内部状態がある場合だけ導入する。高cardinalityのラベルや
ユーザーIDを出力しない。

## ローカル検証

サーバーを変更しない範囲では、次を実行する。

```shell
task --list
task test-tools
task artifacts
task backlog -- validate
task --dry deploy-app
task --dry deploy-all
```

`task --dry deploy-*`では、role、転送元・転送先、activation順を確認する。実際の`task deploy-*`、
`task apply-roles`、`task before-bench`はサーバー状態を変更するため、対象と影響を確認してから実行する。

## 設定完了条件

- Taskfileのホスト、役割、IP、service、port、build、ベンチコマンドが実環境と一致する。
- deploy dry-runの転送先とactivation順に意図しない対象がない。
- collectorが読むログと、nginx・DBが実際に出すログのpath・形式が一致する。
- `task artifacts`で成果物宣言と分析・dashboardの読み手が一致する。
- optional機能は、有効化理由、負荷、停止・cleanup手順が確認できている。
- baseline RUNで必要な成果物が同じRUN IDへ集まり、欠損理由が`run.json`へ記録される。

