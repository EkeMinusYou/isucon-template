# Tools

ビルド・配布・ベンチ・RUN管理・分析の共通ツールです。競技の通常処理はGoだけで動かし、これらの道具は計測・操作用に使います。

## 構成

- `measurement/app`：Goのprofile・runtime・内部指標の公開。
- `measurement/cmd/proc-metrics`：ホスト・Goサービス・disk・任意のtask状態。
- `measurement/cmd/runtime-metrics`：1秒間隔のruntime・内部指標。
- `measurement/cmd/json-metrics-collector`：必要な場合だけ有効化する追加集計。
- `measurectl`：宣言に基づく計測・回収・集計・品質検査。
- `deployctl`：Goアプリ・systemd・OS設定の配布、初期化、役割確認。
- `portal-bench`・`bench-output`：ポータル操作と公式結果の保存・解釈。
- `analysisctl`・`analysis`：保存済みRUNの負荷期間をDuckDBへ同期・比較。
- `dashboard`：保存済みRUNの任意の可視化。
- `user-transition-metrics`・`topology-screen`：アクセスログの操作遷移・分割候補の分析。
- `contest`：当日の設定とadapter。共通ツールへ競技固有の分岐を埋め込まない。

準備・実装の分担は[setup](../.agents/skills/isucon-setup/SKILL.md)と[bootstrap](../.agents/skills/isucon-bootstrap/SKILL.md)に従います。

## 設定項目

### Taskfile

`Taskfile.yml`がホスト・IP・service・port・パスの正本です。`Taskfile.contest.yml`は同じ名前空間に取り込み、共通側のhookである`prepare-initial-data`・`build-contest-assets`・`gen-contest-routing`へ必要な処理を追加します。名前は重複できません。

### デプロイ

`tools/deployctl/deployments.yaml`が配布と実行順を定義します。`deploy-all`は停止対象unitを無効化してからGoアプリを配布・起動します。`initialize`は稼働中アプリの初期化APIを呼びます。並行処理の停止・初期状態の公開・全ホストの同期はGoの初期化処理で実装してください。

`local`・`remote`には`{host}`を指定でき、ホスト別設定を配布できます。`validate`付きuploadは転送先を退避し、検査失敗時にそのuploadを復元します。`DEPLOYCTL_CONFIG`で競技固有の設定を選び、`include`で共通graphを再利用できます。新たなGoサービスのrole・実行・状態確認も同じ設定へ追加します。

### 計測と集計

`measurectl/collectors.yaml`と`digesters.yaml`が生成・回収の正本です。Goアクセスログの出力先は`ACCESS_LOG_PATH`、roleは`app_traffic`です。標準profile、runtime、proc、journal、alpと操作遷移を収集します。`enabled_by_default: false`の計測は必要な場合だけ有効にします。

計測を変えるときは生成側、宣言、回収側、分析側、dashboardの読者を揃え、`task artifacts`で照合します。RUN開始時の成果物契約は`run.json`へ保存し、後日の設定変更で必須対象を変えません。欠損・空・解析失敗・対象外は区別します。

### access log

[Goアクセスログ](../docs/measurement/go-access-log.md)にJSON列・時刻・識別方法をまとめています。API分類は`alp.yml`、`tools/contest/user-transition-routes.json`、`analysis/schema/access-log.sql`で揃えます。生ログはRUNごとに保存・圧縮し、負荷期間を選んでからDuckDBへ集計します。

### RUNの比較

保存済みの2RUNを比較する。新しいベンチは実行しない。

```sh
task compare-runs BASE_RUN=20260101-120000 NEW_RUN=20260101-121000
```

検査・正規deployを済ませた変更を新しく測定し、回収後に比較したい場合は次を使う。

```sh
task bench BASE_RUN=20260101-120000
```

`BASE_RUN`は比較元の過去RUNを指定する。ベンチの実行条件には使わない。省略時は従来どおりベンチと回収だけを行う。指定したRUNが未確定・未同期ならベンチ開始前に停止する。失敗RUNも通常回収し、可能なら比較を表示する。回収・比較の失敗も終了コードへ反映し、自動で再投入しない。

比較では合否・得点差、構成・負荷時間窓、計測の欠損・不備、主要20経路の件数・応答時間・HTTPエラー、ホストのCPU・メモリを表示する。欠測は`NULL`で残す。経路集計は負荷区間内に開始する5秒バケットを使い、境界には最大5秒のずれがある。AIは比較元と仮説固有の追加観測、変更の採否を決める。時間窓・構成の違いや計測の不備も確認し、標準比較だけで評価を済ませない。

共通SQLは`tools/analysis/sources.yaml`の`reports.compare_runs`で指定する。競技固有の標準集計を比較する場合はSQLを`tools/contest/queries/`に置き、`reports.compare_contest`へ指定する（任意）。bootstrapで新実装の共通比較を確認し、選んだ情報源に合わせて競技固有の比較を追加する。

両RUNは確定・DuckDB同期済みである必要がある。同期失敗は`task q-sync`で補修し、`task compare-runs`で比較だけをやり直す。

### 分析

`task q`は読み取り専用、`task q-sync`はfinalized RUNの取り込みを行う。
同期中に別のDuckDBセッションとファイルロックが競合した場合は、最大60秒待って再試行する。
待機期限を超えた場合は該当セッションを閉じて`task q-sync`を再実行する。回収済みRUNのベンチ再実行は不要。

`tools/analysis/sources.yaml`は、RUN成果物とDuckDB schemaの対応表である。標準成果物だけなら変更しない。
成果物名・列・DB種別を変更する場合は、次の宣言と読み手（queryを含む）を一つの変更で揃える。

1. collectorまたはdigesterの出力宣言
2. `run.json`へ記録されるartifact契約
3. `analysis/sources.yaml`のsource
4. 読み込みschemaとsemantic view
5. 必要なDuckDBのquery
6. dashboardで表示する場合はAPIと表示

ベンチ出力の結果・内訳・進捗は形式が競技ごとに違うので、汎用の`analysis/schema/bench-summary.sql`は拡張せず、
`tools/contest/analysis-schema/`にviewとして作る。詳細は[tools/contest/README.md](contest/README.md)。

状態・操作の集計と保存は[状態・操作の計測](../docs/measurement/state-and-operations.md)を参照する。

### Go profileのDuckDB閲覧

標準収集は`{host}-go-cpu.pprof`と`{host}-go-snapshots.tar`をRUNへ保存する。
既存DBはschema version変更時に自動再構築する。元の成果物は変更しない。
採取方法・限界・上限は[Go profile](../docs/measurement/go-profiling.md)、
他の計測を含む期間の扱いは[標準計測の一覧](../docs/measurement/README.md#標準計測で答えられる問い)を参照する。

| table / view | 内容 |
|---|---|
| `go_runtime_series` / `go_runtime_window` | 1秒のruntime指標と期間集計。counterは再起動をまたがない差分・rate |
| `go_profile_recordings` | recordingの完全性、採取数、設定間隔、最大export所要時間 |
| `go_profile_points` | 負荷期間内の時刻・sample種別・単位・総量・valid。allocationは隣接時点の差分 |
| `go_profile_point_functions` | 時点別・関数別flat/cumulative値 |
| `go_profile_function_window` | 状態の平均・最大、allocation差分の合計・毎秒量 |
| `go_profile_import_scope` | 個別pprofの取り込み可否と理由 |
| `measurement_sources` | 期間を分離できないsourceの除外理由 |

```sh
task q -- "SELECT * FROM go_runtime_window WHERE run_id='RUN_ID' ORDER BY host,metric;"
task q -- "SELECT host,profile_type,sample_type,value_unit,make_timestamp_ns(captured_at_ns) AS captured_at,total_value,valid FROM go_profile_points WHERE run_id='RUN_ID' ORDER BY host,captured_at,profile_type,sample_type;"
task q -- "SELECT host,function,mean_observed,max_observed FROM go_profile_function_window WHERE run_id='RUN_ID' AND profile_type='go-goroutine' AND sample_type='goroutine' ORDER BY mean_observed DESC LIMIT 20;"
task q -- "SELECT * FROM go_profile_recordings WHERE run_id='RUN_ID';"
```

関数が存在しない時点も平均の母数に含める。goroutineの値は観測された個数であり、待ち秒数ではない。
`inuse_space`はbytes、`alloc_space_delta`は区間内の割当bytes、`alloc_objects_delta`は個数。
時点の量を合計して消費量とみなさない。RUN・host・profile_type・sample_type・単位を揃えて比較する。
allocationの負の差分は`valid=false`で集計から除外する。GCによる反映遅れがあるためruntime counterも併せて読む。

既存の単体pprofは採取時刻とdurationが負荷期間内に収まる場合だけ`pprof_metadata`、
`pprof_samples`、`pprof_frames`、`pprof_functions`、`pprof_edges`へ展開する。
heapの累積allocationと旧allocs profileは負荷区間の量にできないため除外する。
連続snapshotの時系列は上記DuckDBのqueryで読む。dashboardの単体pprof表示は過去のsnapshot用として残す。
通常の全期間CPUはRUN上で`go tool pprof -top runs/RUN_ID/HOST-go-cpu.pprof`として読む。
`profile_*`と`bottleneck_profile_*`、`analysisctl profile`はfgprof専用で、同じ期間条件に従う。
不明frameは`[unknown]`。binary・ソースの一致を自動保証せず、欠測は0とみなさない。

### アプリ固有adapter

`tools/contest/bench-patterns.json`のscore・pass・fail正規表現を当日のベンチ出力形式へ合わせる。
coreはログ形式を持たず、下流が読む`SCORE:`・`BENCHMARK_PASS`・`BENCHMARK_FAIL`は競技によらず固定である。
scoreには1つのcapture groupを持たせ、該当する行がない項目は空文字にする。
全項目が空でも原文とコマンドの終了コードを保持し、結果markerは追加しない。
出力書式が未確定でも開始・回収でき、スコアや成功は推測しない。宣言が読めない場合は
ベンチを起動する前に終了コード2で停止する。

`tools/contest/user-transition-routes.json`のCookie列・API prefix・正規化routeを当日のAPIへ合わせる。
bootstrapでは[重要な操作間の標準集計](../docs/measurement/state-and-operations.md#重要な操作間の標準集計)も設定する。
`task q`で`operation_progress`と`connection_reuse`を比較できる。routeは動的IDを
そのまま残さず、限定された正規表現を上から具体的な順に並べる。Cookieを持たない競技や、識別子を
安全にログへ出せない場合は、この集計を無効にする。access logの時系列bucket
（`tools/analysis/schema/access-log.sql`の`# >>> contest values >>>`区画）と`alp.yml`の
`matching_groups`にも同じ正規化を入れる。

`tools/contest/analysis-schema/bench-errors-semantic.sql`の`bench_errors`・`bench_warning_events`・
`bench_error_counts`を当日のベンチ出力の書式へ合わせる。bench.logの読み込み自体は汎用の
`tools/analysis/schema/bench-errors.sql`が行い、ここは行の解釈だけを持つ。view名と列はdashboardと
`task q`の契約なので変えない。書き換えるまでは0行を返す。

`tools/measurement/cmd/json-metrics-collector`は、標準では利用しない。既存のHTTP・runtime・profile計測では見えない、
スコアに直接関係するboundedなアプリ内部状態がある場合だけ導入する。高cardinalityのラベルや
ユーザーIDを出力しない。利用時はアプリ側にboundedなenable/snapshot/disable endpointを実装し、metric・scope・上限を決めてcollector宣言を追加する。


## 検証コマンドの意味

- `task setup-check`は`task test-tools`、`task artifacts`とdeploy dry-runを含む。個別検査と全体検査が重なる場合、通過済みの検査は変更・失敗・未解決の懸念がない限り繰り返さない。
- `task deploy-*-dry`はdeployctlの`-dry-run`でrole・転送元/先・activation順を検証する。Go Taskの`task --dry`は表示のみで、代用できない。
