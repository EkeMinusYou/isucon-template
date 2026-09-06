# Tools

`tools/`には、デプロイ、ベンチ計測、RUN管理、分析、可視化を行う再利用可能なツールを置く。
ツール本体は大会固有のアプリケーションへ依存させず、当日の差分は設定ファイル、Taskfile変数、
実行引数、必要な場合だけアプリ側の計測endpointで吸収する。

## 基本方針

- 公式資料と実環境を確認してから設定する。過去大会の値をそのまま採用しない。
- ツール本体を書き換える前に、設定ファイルまたは実行引数で表現できないか確認する。
- collectorを増やす場合は、成果物の宣言・回収・分析側の読み手を同じ変更で揃える。
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
| `setup/` | 取得・構文検査の調整時 | [取得対象の除外、schemaの実体取得、設定構文検査の実装例](setup/README.md) |
| `proc-metrics/` | service設定 | Linuxの`/proc`、`/sys`、cgroup v2を使う。`-services`へ実際のsystemd unitを渡し、sampling intervalを確認する |
| `topology-screen/` | 利用時に引数調整 | 配置候補を調べる場合だけ、key field・regex、route、hash、分割数、対象bucketを当日のデータモデルへ合わせる |
| `user-transition-metrics/` | adapter設定 | [アプリ固有adapter](#アプリ固有adapter)・[access log](#access-log) |

## 設定項目

### Taskfile

ホスト、役割、IP、アプリ名、DB名、service、port、build、ベンチコマンドを実環境へ合わせる。OS・CPU architectureと、service・設定・ログのパスを確認して値を決める。

`MYSQL_HOSTS`はdeploy・role収束・状態検査の対象、`MYSQL_HOST`は詳細計測する1台です。
両者の既定値は同じです。計測の`mysql` roleは1台のまま、全配置は`mysql_all` roleとしてRUNへ記録します。
MySQLの共通設定とlimitsは`MYSQL_HOST`から取得します。ホスト別設定がある場合は一律配布せず、
次節の`{host}`による転送元の分離を使ってください。

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

設定uploadに`validate: sudo nginx -t`のような検査コマンドを指定すると、配布先を退避してから
その場へ上書きし、検査します。転送または検査の失敗時は、そのuploadの配布先を元に戻します。
すべてのuploadと検査が成功するまでactivationは開始しません。nginx/MySQLの標準deployにも設定済みです。
検査コマンドは実サービスが読む設定とインストール済みversionに合わせてください。

退避先はホスト上の`/tmp/isucon-deployctl-<ID>`で、通常終了時は削除します。
SSH切断・復元失敗などで残った場合は、表示された退避先と終了状態を確認して復旧します。
復元対象は失敗したuploadだけです。他ホストで成功した配布、別upload、activation後の失敗は自動で戻しません。
同じ配布先への並行deployを避け、必要ならgit上の旧設定から正規deployで戻してください。

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

標準設定ではproc/service/disk、MySQL status、nginx access log、slow query、app/nginx/kernel journal、Go pprof・fgprof、user-transitionを収集する。初期setupでadapterを整え、分析・dashboardまで確認する。journalは`run.json.load_window`と同じ時間窓で回収できることを確認する。
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

標準のfgprof・Go CPU・heap・allocs・goroutineは`group: profiles`かつ`enabled_by_default: true`。
`bench/run.sh`がこの宣言から収集対象を選び、同じ対象を開始確認・必須成果物判定で使う。
[Go profile導入例](../docs/special-sources/go-profiling.md)に沿ってsetupでendpointを用意する。
CPUとfgprofのRUN別開始確認後にベンチを開始し、`PROFILE_SECONDS`秒の収集・回収完了後にfinalizeする。
開始通知はprofilerの起動成功後に公開する。開始確認後は、CPU profile writerの非同期起動と
小さなホスト時計差に備えて1秒先行収録し、同じRUNが収録中であることを再確認してからベンチを開始する。
この1秒は大きな時計差を補正するものではなく、成果物の時間窓検査は緩和しない。
時間・snapshotの遅延・HTTP timeout・開始確認timeoutはTaskfileで競技に合わせる。
endpointはloopbackで公開する。`PROFILES_ENABLED=false`またはno-collectorsタスクで停止できる。
各RUNの`required_artifacts`に有効なホスト別profile・圧縮access log・user-transitionを固定し、内容・計測窓・欠損を検査する。
`artifact_contract`には開始時の成果物宣言も保存し、途中で既定値が変わってもRUNの検査条件を維持する。
raw配下の個別ファイルもmanifestに記録し、trafficのないホストの空ログも圧縮・保存する。
過去のRUNへ新しい必須条件を遡及適用しない。CPU・fgprof同時取得の計測負荷は比較RUNで評価する。

### access log

標準集計では、nginxのJSON access logに次の列を用意する。

- 共通: `msec`、`method`、`uri`、`status`、`response_time`、`body_bytes`
- upstream分析: `upstream_time`、`upstream_addr`、`upstream_status`、`cache_status`
- ユーザー遷移: Cookie由来の専用識別列。既定名は`session_id`

識別値は個人情報を含めず、認証Cookie等の生値を恒久保存しない。集計成果物には識別値やhashを保存しない。
user-transitionは標準対象であり、setupで識別列とアプリ固有API分類を整える。
有効なAPI入力・識別情報がない成果物は検査失敗となる。未設定をoptional扱いにしない。
仕様上成立しない場合だけ、根拠を残して`enabled_by_default: false`とする。

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

### Go profileのDuckDB閲覧

finalized RUN直下の `{host}-fgprof.pprof`、`{host}-go-cpu.pprof`、
`{host}-go-heap.pprof`、`{host}-go-allocs.pprof`、`{host}-go-goroutine.pprof` を
`task q` が取り込む。既存DBは取り込みschema versionの変更時に自動再構築する。
RUNのない疎通確認ファイルは取り込まない。取得完了を待ってからRUNをfinalizeする。

| table | 内容 |
| --- | --- |
| `pprof_metadata` | profile種別・sample種別・単位、SHA-256、採取時刻、duration、総量、sample数、不完全なstack数 |
| `pprof_samples` | stackごとの値、変換前の整数値 `raw_value`、stack深さ |
| `pprof_frames` | sampleごとのleafからrootへのframe列 |
| `pprof_functions` | 関数別 `flat_value` と `cumulative_value` |
| `pprof_edges` | caller/callee別の値と出現数 |

`profile_type` は `fgprof` / `go-cpu` / `go-heap` / `go-allocs` / `go-goroutine`。
複数のsample種別をすべて保持する。CPUは `sample_type='cpu'`、fgprofは `time`、
heapの使用中メモリは `inuse_space`、累積allocationは `alloc_space`、
goroutine数は `goroutine` を選ぶ。heap/allocsは両方のprofileに使用中・累積の値が含まれるため、
同じ意味の値を複数profileから足し合わせない。

時間の `value_unit` は `seconds`、容量は `bytes`、個数は `count`。
`sample_unit` と `raw_value` は元の単位・整数値を保持する。
CPU秒、goroutineのwall-clock秒、バイト数、個数は互換ではない。
必ずRUN・host・profile_type・sample_typeを選んでから比較・集計する。
table間のjoinには `(run_id, host, source, sample_type)`、sampleとframeのjoinにはさらに `sample_id` を使う。
`duration_seconds` は採取窓であり、総CPU時間・全goroutineの時間合計とは異なる。
snapshotのduration=0は正常。採取時刻 `time_unix_nano=0` は不明として扱う。

取得状況:

```sh
task q -- "SELECT run_id, host, profile_type, sample_type, value_unit, total_value, sample_count, duration_seconds, make_timestamp_ns(nullif(time_unix_nano, 0)) AS captured_at FROM pprof_metadata ORDER BY run_id DESC, host, profile_type, sample_type;"
```

CPUの重い関数（RUN_IDは実際のRUNへ置換）:

```sh
task q -- "SELECT function, flat_value AS cpu_seconds, cumulative_value FROM pprof_functions WHERE run_id='RUN_ID' AND host='isucon-1' AND profile_type='go-cpu' AND sample_type='cpu' ORDER BY flat_value DESC LIMIT 20;"
```

使用中メモリの大きい関数:

```sh
task q -- "SELECT function, flat_value AS inuse_bytes, cumulative_value FROM pprof_functions WHERE run_id='RUN_ID' AND host='isucon-1' AND profile_type='go-heap' AND sample_type='inuse_space' ORDER BY flat_value DESC LIMIT 20;"
```

サンプルが0件でも `pprof_metadata` は存在する。ファイルの欠損とは区別し、
`run.json` / `artifacts` の状態も確認する。不正protobufや未知の単位は取り込みエラーとなり、
再構築に失敗した場合は旧DBを保持する。関数名はprofileに埋め込まれた情報を使い、
不明frameは `[unknown]` とする。別binaryによる補完やソース一致の保証は行わない。

既存の `profile_*` と `bottleneck_profile_*` はfgprofのwall-clock秒専用の互換table/viewとして維持する。
`analysisctl profile` の既存レポートもfgprof専用。標準pprofは上記 `task q` で閲覧する。

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
