# Tools

`tools/`には、デプロイ、ベンチ計測、RUN管理、分析、可視化を行う再利用可能なツールを置く。
ツール本体は大会固有のアプリケーションへ依存させず、当日の差分は設定ファイル、Taskfile変数、
実行引数、必要な場合だけアプリ側の計測endpointで吸収する。

## 基本方針

- 公式資料と実環境を確認してから設定する。過去大会の値をそのまま採用しない。
- ツール本体を書き換える前に、設定ファイルまたは実行引数で表現できないか確認する。
- 汎用ファイルへ競技固有の値を書く場合は、直前の行へ`# contest:`から始まるコメントで理由を残す。構造そのものが特定の解法に依存する場合は、値ではなくファイルを分ける。次の競技へ引き継ぐ際は`grep -rn '# contest:' tools/`が要修正箇所の一覧になる。
- 競技ごとに書き換える値がまとまっている場合は、`# >>> contest values >>>` 〜 `# <<< contest values <<<`で囲う。`Taskfile.yml`のvarsがこれにあたる。手段の使い分けとtemplateへの戻し方は[tools/template/README.md](template/README.md)。
- collectorを増やす場合は、成果物の宣言・回収・分析側の読み手を同じ変更で揃える。
- 計測設定の評価ではcollectorあり・なしを比較し、間隔や有効化範囲を決める。この負荷比較は初期setupの完了条件には含めず、依頼された場合に`isucon-setup`の追加検証としてユーザーが実行したベンチ結果を使う。
- optionalなcollectorやprofilerは、目的と停止・cleanup手順が確認できた場合だけ有効にする。
- deploy、service restart、ログローテート、collector起動はサーバー状態を変更する。対象を確認してから実行する。

このテンプレートの既定構成は、Linux、systemd、cgroup v2、Go、nginx、MySQL、`linux/amd64`である。
異なる構成でも各ツールのcoreを流用できるが、対応するadapterやTaskfileを変更する必要がある。

## 読み方

作業順・service分類・完了判断は[isucon-setup](../.agents/skills/isucon-setup/SKILL.md)を正本とする。本書はツール固有の設定資料であり、下表で対象を選び、詳細節を読む。標準外構成も管理対象に含め、optional機能の詳細は利用時だけ読む。

既存環境の計測不足の調査・修正・検証は[isucon-setup](../.agents/skills/isucon-setup/SKILL.md)の部分補修手順に従う。

## ディレクトリ別チェックリスト

| ディレクトリ | 当日の対応 | 確認・変更するもの |
| --- | --- | --- |
| `analysis/` | 成果物・DB・スコア変更時 | [分析](#分析) |
| `analysisctl/` | 原則不要 | `analysis/sources.yaml`を読む汎用core。DuckDB CLIとRUNディレクトリが利用できることを確認する |
| `bench/` | 一部確認 | `run.sh`が自動・手動ベンチ共通のload windowとfinalizeを担う。nginx on-CPU profilerを使う場合はOS、package manager、kernel用`perf`、probe URLを確認する |
| `bench-output/` | ログ形式確認 | ベンチ出力からscore・合否を読む汎用core。形式は`tools/contest/bench-patterns.json`に分離する。[アプリ固有adapter](#アプリ固有adapter) |
| `browser/` | 原則不要 | ローカルdashboard確認専用。必要な場合だけ`task browser-install`を実行し、競技サーバーへ配布しない |
| `contest/` | 毎回入れ替え | この競技だけで成立する設定・ツールの集約先。他のディレクトリはここを参照する側でも競技非依存に保つ。[一覧と置く基準](contest/README.md) |
| `dashboard/` | 条件付き | 標準成果物なら変更不要。独自collector、独自スコア列、追加ロールを表示する場合だけserver APIとweb UIを拡張する |
| `deployctl/` | 毎回確認 | [デプロイ](#デプロイ) |
| `json-metrics-collector/` | 利用時のみ実装 | [アプリ固有adapter](#アプリ固有adapter) |
| `measurectl/` | 毎回確認 | [計測と集計](#計測と集計) |
| `mysql-metrics/` | 接続・互換性確認 | `-dsn`、認証、socket/port、MySQL version、`performance_schema.data_lock_waits`の利用可否を確認する |
| `nginx-backend-report/` | log列確認 | [access log](#access-log) |
| `nginx-oncpu-profiler/` | 利用時のみ環境調整 | 既定では無効。`perf`権限、kernel package、worker数、delay、duration、frequency、出力上限を確認する |
| `setup/` | 取得・構文検査の調整時 | [取得対象の除外、schemaの実体取得、設定構文検査の実装例](setup/README.md) |
| `template/` | 大会後 | templateへ戻す差分の宣言と抽出。当日は変更しない。[backportの手順](template/README.md) |
| `proc-metrics/` | service設定 | Linuxの`/proc`、`/sys`、cgroup v2を使う。`-services`へ実際のsystemd unitを渡し、sampling intervalを確認する |
| `sql-pool-metrics/` | アプリadapter確認 | [collector README](sql-pool-metrics/README.md)に沿ってendpoint契約を合わせる |
| `topology-screen/` | 利用時に引数調整 | 配置候補を調べる場合だけ、key field・regex、route、hash、分割数、対象bucketを当日のデータモデルへ合わせる |
| `user-transition-metrics/` | adapter設定 | [アプリ固有adapter](#アプリ固有adapter)・[access log](#access-log) |

## 設定項目

### Taskfile

ホスト、役割、IP、アプリ名、DB名、service、port、build、ベンチコマンドを実環境へ合わせる。OS・CPU architectureと、service・設定・ログのパスを確認して値を決める。

競技固有・解法固有のタスクは`Taskfile.contest.yml`へ置き、`Taskfile.yml`が`flatten: true`で取り込む。
取り込んだタスクは名前空間なしで呼べ、`Taskfile.yml`のvarsとタスクをそのまま参照できる。
ただし**同名タスクは上書きではなく衝突エラーになる**ため、汎用側から競技側を呼ぶ場合は、
汎用側にhookタスク名を書き、実体を`Taskfile.contest.yml`で定義する。既定では
`deploy`系が`prepare-db-assets`を、`gen`が`gen-contest-routing`をhookとして呼ぶ。
該当する処理がない競技では、hookをcmdsなしで定義すれば何もせず成功する。
deployctlの`include`とは規則が異なる（あちらは同名を後勝ちで置換する）ので取り違えない。

ホスト台数は`ALL_HOSTS`・`APP_HOSTS`・`MYSQL_HOSTS`などのリストで決まり、role配布、
分割数、ホスト別生成物は台数へ追従する。nginxは算術を持たないため、ユーザーIDの剰余などで
upstreamを選ぶmapを作る場合だけは台数を固定した正規表現になる。この制約は
`gen-contest-routing`側に閉じ込める。

`MYSQL_HOSTS`はdeploy・role収束・状態検査の対象、`MYSQL_HOST`は既定接続先です。
MySQL status、slow log、performance_schema digest、lock waitは`mysql_all`の全ホストを対象にし、
成果物は`<host>-mysql-status.tsv`、`<host>-slp.tsv`、`<host>-mysql-digest.tsv`、
`<host>-mysql-lock-waits.tsv`として保存します。`MYSQL_HOST`は後方互換の既定接続先と、
旧RUNのhost解決に残ります。MySQLの共通設定とlimitsは`MYSQL_HOST`から取得します。
ホスト別設定がある場合は一律配布せず、次節の`{host}`による転送元の分離を使ってください。

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

宣言の構造そのものが特定の解法に依存する場合は、`deployments.yaml`を書き換えず別ファイルへ分ける。
`include:`へ相対パスを並べると読み込んだ側が勝つので、汎用グラフをincludeする側に解法固有の宣言を置く。
同名のdeploymentとplanは置き換わり、触れていないものはincludeから引き継ぐ。`DEPLOYCTL_CONFIG`が
実際に使う宣言を指す。例えば`tools/contest/deployments.yaml`が汎用graphをincludeし、
解法固有の`db-schema`だけを置き換える。

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

標準設定ではproc/service/disk、appのSQL接続プール、全MySQL hostのstatus・slow query・performance_schema digest、nginx access log、app/nginx/kernel journal、Go pprof・fgprof、user-transitionを収集する。初期setupでadapterを整え、分析・dashboardまで確認する。journalは`run.json.load_window`と同じ時間窓で回収できることを確認する。
MySQLのslow log、Performance Schema、socket接続の前提は[MySQLの計測データ](../docs/measurement/mysql.md)を参照する。
50msのlock waitは各user DBを対象に既定有効で、250msのtask stateは既定無効である。
lock waitはホスト別の成果物へ保存し、RUNごとにcollector負荷とcapture errorを確認する。

digesterも`enabled_by_default: false`で既定の実行対象から外せます。省略時は従来どおり有効です。
無効なdigesterの出力は任意成果物となり、未生成でも必須成果物の欠損にはなりません。
`measurectl digest -only <name>`で明示的に実行できます。sourceのログ回収は独立しており、この設定では停止しません。

`MEASURECTL_ROLES`に追加した任意のrole（例: `-role cache=host-a,host-b`）は、
`run.json`の`roles.additional`に保存されます。分析の`manifests.additional_roles`から参照できます。
既存の標準roleフィールドと`scores.tsv`形式は維持します。
直接`manifest begin`を使う場合は`-role cache=host-a,host-b`を渡してください。
古いRUNの未記録roleは推測で補いません。

標準のfgprof・Go CPU・heap・allocs・goroutineは`group: profiles`かつ`enabled_by_default: true`。
`bench/run.sh`がこの宣言から収集対象を選び、同じ対象を開始確認・必須成果物判定で使う。
[Go profile導入例](../docs/measurement/go-profiling.md)に沿ってsetupでendpointを用意する。
CPUとfgprofのRUN別開始確認後にベンチを開始し、設定した時間の収集・回収完了後にfinalizeする。
開始通知はprofilerの起動成功後に公開する。開始確認後は、CPU profile writerの非同期起動と
小さなホスト時計差に備えて1秒先行収録し、同じRUNが収録中であることを再確認してからベンチを開始する。
この1秒は大きな時計差を補正するものではなく、成果物の時間窓検査は緩和しない。
開始・終了マーカーのUTC時刻は`task bench-timestamp`で`ENTRY_HOST`から取得する。
ローカルPCの時計をサーバーのprofile・アクセスログと混ぜない。時刻取得に失敗した場合は
マーカーを出さずにエラーとし、通常のfinalize処理で取得済み成果物を保存する。
競技サーバー同士の時計差は別途確認する必要がある。
時間・snapshotの遅延・HTTP timeout・開始確認timeoutはTaskfileで競技に合わせる。
endpointはloopbackで公開する。`PROFILES_ENABLED=false`またはno-collectorsタスクで停止できる。
各RUNの`required_artifacts`に有効なホスト別profile・圧縮access log・user-transitionを固定し、内容・計測窓・欠損を検査する。
`artifact_contract`には開始時の成果物宣言も保存し、途中で既定値が変わってもRUNの検査条件を維持する。
raw配下の個別ファイルもmanifestに記録し、trafficのないホストの空ログも圧縮・保存する。
過去のRUNへ新しい必須条件を遡及適用しない。CPU・fgprof同時取得の計測負荷は比較RUNで評価する。

### access log

nginx側のJSON列、識別情報の扱い、設定反映は[nginx access logの計測手順](../docs/measurement/nginx-access-log.md)を参照する。
本書ではaccess logを回収するsource・digesterと、分析schemaとの対応を管理する。

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

`tools/contest/bench-patterns.json`のscore・pass・fail正規表現を当日のベンチ出力形式へ合わせる。
coreはログ形式を持たず、下流が読む`SCORE:`・`BENCHMARK_PASS`・`BENCHMARK_FAIL`は競技によらず固定である。
scoreには1つのcapture groupを持たせ、該当する行がない項目は空文字にする。宣言が読めない場合は
ベンチを起動する前に終了コード2で停止する。

`tools/contest/user-transition-routes.json`のCookie列・API prefix・正規化routeを当日のAPIへ合わせる。routeは動的IDを
そのまま残さず、限定された正規表現を上から具体的な順に並べる。Cookieを持たない競技や、識別子を
安全にログへ出せない場合は、この集計を無効にする。access logの時系列bucket
（`tools/analysis/schema/access-log.sql`の`# >>> contest values >>>`区画）と`alp.yml`の
`matching_groups`にも同じ正規化を入れる。

`tools/contest/smoke`のリクエストフローを当日のセッションフロー（登録・ログイン・認証つきの読み取り数本）へ
合わせる。`task setup-smoke`が`TARGET_OS`/`TARGET_ARCH`へビルドし、ENTRY_HOSTへ配置してloopbackに対して実行する。
ベンチを使わずにaccess log・slow query・profile・user-transitionが出ることを確認するためのものなので、
更新は最小限にとどめ、セッション識別子や認証情報を出力しない。

`tools/contest/analysis-schema/bench-errors-semantic.sql`の`bench_errors`・`bench_warning_events`・
`bench_error_counts`を当日のベンチ出力の書式へ合わせる。bench.logの読み込み自体は汎用の
`tools/analysis/schema/bench-errors.sql`が行い、ここは行の解釈だけを持つ。view名と列はdashboardと
`task q`の契約なので変えない。書き換えるまでは0行を返す。

`tools/json-metrics-collector`は、標準では利用しない。既存のHTTP・DB・profile計測では見えない、
スコアに直接関係するboundedなアプリ内部状態がある場合だけ導入する。高cardinalityのラベルや
ユーザーIDを出力しない。利用時はアプリ側にboundedなenable/snapshot/disable endpointを実装し、metric・scope・上限を決めてcollector宣言を追加する。

`tools/sql-pool-metrics`は、アプリのlocalhost debug endpointから`database/sql.DBStats`相当の接続プール状態を
1秒間隔で収集する汎用collectorである。アプリ側は固定されたpool名・role・shardだけを返し、リクエストやユーザー識別子を返さない。
実装するendpointの契約は[`tools/sql-pool-metrics/README.md`](sql-pool-metrics/README.md)を参照する。
`in_use`、`wait_count`、`wait_duration`とその差分レートをMySQL側の`Threads_running`やCPUと同じ時間窓で比較し、
接続プール待ちとDBサーバー飽和を切り分ける。
MySQL status collectorはこの比較用に`max_connections`、`Max_used_connections`、
`Connection_errors_max_connections`も収集する。

## 検証コマンドの意味

- `task setup-check`は`task test-tools`、`task artifacts`とdeploy dry-runを含む。個別検査と全体検査が重なる場合、通過済みの検査は変更・失敗・未解決の懸念がない限り繰り返さない。
- `task deploy-*-dry`はdeployctlの`-dry-run`でrole・転送元/先・activation順を検証する。Go Taskの`task --dry`は表示のみで、代用できない。
