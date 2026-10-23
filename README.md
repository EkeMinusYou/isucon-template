# ISUCON template

ISUCONの競技サーバーを、ローカルのリポジトリを正本としてセットアップ・デプロイし、
ベンチ1回ごとのログ・メトリクス・profileを同じ`RUN_ID`へ回収するためのテンプレートです。

改善活動の最上位目的は、公式ルールと正当性要件を満たしたうえで、最終スコアを最大化することです。

競技中の変更は[ISUCONでの変更方針](AGENTS.md#isuconでの変更方針)に従います。
依頼された改善に必要なDB再作成や不要ファイルの削除などの破壊的変更を許容し、
切り戻し専用の仕組みや旧実装の保持は求めません。公式の正当性・永続性・追試要件は維持します。

templateの既定は、`isucon-1`がアプリ・nginx・MySQLを兼ねる単一ホスト構成です。
ホストの役割分担は競技開始時に`Taskfile.yml`の`*_HOSTS`で設定します。

ベンチは`task bench`で実行します。設定したポータルへジョブを投入し、ログと計測結果を自動回収します。
通常回収では失敗RUNも保存し、分析DBを同期します。`after-bench`は成果物をローカルGit commitします。

`task db-recreate`は`MYSQL_HOST`の競技DBを破棄して公式schemaから作り直します。
アプリの書込みを止め、schema再作成、アプリ起動、`INITIALIZE_PATH`への初期化リクエストの順に処理します。
通常の`task deploy-all`はDBを再作成しません。

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

構成の正本は`Taskfile.yml`です。`# >>> contest values >>>`区画のホスト・役割・内部IP・service・DB・TARGET_OS/ARCHを
実機と公式資料に合わせて設定してから、取得・生成・検査を行います。

`~/.ssh/config`に同じホストaliasを設定し、読み取りで実環境と公式資料を確認してから取得します。

```shell
task
task inspect-hosts
task setup
task gen
task setup-check
```

`setup-webapp`の取得対象は`SETUP_WEBAPP_EXCLUDES`で指定したrsync除外ファイルで調整できます。
Goの生成binaryと他言語の取得済み生成物を除外しています。schemaのsymlink、
実サーバーでの設定構文検査は[取得と構文検査の手引き](tools/setup/README.md)を参照してください。

`MYSQL_HOSTS`はDB設定配布とservice有効化の対象、`MYSQL_HOST`は標準メトリクス・対話DB接続・既定のDB接続先です。
DBをアプリと別ホストへ置く場合は、アプリが`MYSQL_HOST`の内部IPへ接続するよう設定します。
`task gen`が接続先を生成し、ホスト固有の認証情報は既存の`env.sh`に保持します。

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
task deploy-all      # DB・アプリ・nginxを依存順に反映。DB初期化はしない
task db-recreate     # アプリを止め、DBを破棄・再作成して初期化
task apply-roles     # 役割変更後だけenable/disableを収束
task check-roles
task check-network
```

サーバーを変更せず実際の転送先・role・activationを確認する場合は、Go Task自身の`--dry`ではなく
deployctlのdry-runを呼ぶ次のTaskを使います。

```shell
task deploy-app-dry
task deploy-all-dry
task db-recreate-dry
```

`DEPLOYCTL_CONFIG`（既定`tools/deployctl/deployments.yaml`）が転送・activationのgraphを定義し、`Taskfile.yml`が役割と値を持ちます。
新しいサービスが必要ならdeploymentを追加し、通常の変更に一時的な迂回Taskを増やさないでください。

`db-recreate`は`setup-preflight`でローカルの役割・schema・追加資材とdry-runを確認してから、
アプリを停止し、`MYSQL_HOST`の`DB_NAME`を再作成して`SQL_DIR`の`SQL_SCHEMA_FILE`を適用します。
アプリを起動して`INITIALIZE_PATH`への初期化リクエストを一度実行し、失敗時は後続へ進みません。
`RESET_INPUTS`とSQLのuploadが初期化資材を管理します。`db-recreate-dry`で対象と順序を確認してください。

`task config-check`はnginx/MySQLの正規配布先へ設定を書き込み、構文検査します。reloadは行いませんが、
成功した設定は配布先に残ります。初期化時の`CONFIG_CHECK_COMMAND`へ設定する前に、検査コマンドと
追加service・ホスト別設定の除外／上書き順序を実環境へ合わせてください。
nginxの`tls/`とMySQLの`debian.cnf`は取得・配布から除外し、各ホストに保持します。
別の場所の秘密情報も取得前に確認し、除外対象を調整してください。

`deploy-app-unit`はunitだけを反映してdaemon-reloadし、停止中のアプリを起動します。稼働中プロセスへ
unit変更を適用するには通常の`deploy-app`による再起動が必要です。
`deploy-app-unit-dry`で対象と操作を表示できます。

## ベンチ計測

`Taskfile.yml`の`CONTEST_PORTAL_BENCH_CONFIG`が、ポータルの実行・結果取得設定を指定します。
URL、認証ヘッダー、要求形式、結果の抽出方法と完了条件は設定ファイルで差し替えます。
CookieとCSRFトークンを環境変数へ設定し、実行対象を確認してください。
詳細とISUCON12・13の設定例は[ポータルベンチツール](tools/portal-bench/README.md)を参照してください。

```shell
task portal-bench-check # 設定と認証変数の確認のみ。通信・ジョブ投入なし
task bench
```

`task bench`はポータル設定を事前検証し、配布物とservice状態が一致することを確認してから、
操作ロック内で計測・ジョブ投入・完了待ち・ログ取得・成果物回収を行います。
原文は`benchmark-result.txt`へ保存し、`bench.log`へログと`SCORE:`・`BENCHMARK_PASS`／`BENCHMARK_FAIL`を記録します。
ポータルの結果判定は`outcome`で設定し、不明な成功やスコアは補いません。
回収時のログ変換は`tools/bench-output`と`tools/contest/bench-patterns.json`を使います。

profile自動収集とuser-transitionは標準で有効です。[Go profile導入例](docs/measurement/go-profiling.md)に沿って
setupで全APP_HOSTSへendpointを導入します。[nginx access logの計測手順](docs/measurement/nginx-access-log.md)に沿って
識別列・API分類を整えます。通常の`task bench`で
CPU・heap・allocs・goroutineを自動収集します。fgprofは既定無効で、必要な場合は`task fgprof-collect`で収集します。CPUの開始を確認してから負荷を開始し、
収集・回収完了後にRUNを確定します。追加のprofile操作は不要です。
profile対象は`collectors.yaml`の`group: profiles`と`enabled_by_default`から選びます。
`PROFILES_ENABLED=false`は計測負荷比較等での明示的な停止用です。setupではDuckDB分析まで確認します。

実行前に`pwd`、使用するTaskfile、対象ホストを確認してください。collectorの残存が検出されたら、
そのRUN IDと共有の`raw/current-run-id`を照合します。別checkoutで計測中の可能性があるため、
所有元と終了状態を確認してから対処します。`abort-run`はこの設定が探索するcollectorをまとめて掃除します。
`before-bench`後に中断したRUNを破棄する場合だけ`task abort-run`を使います。通常の回収は必ず`after-bench`です。
`task bench`は、ベンチ失敗や割り込みでも可能な限り`after-bench`を実行し、
失敗RUNをEvidenceとしてfinalizeします。共通の開始・終了・trap処理は`tools/bench/run.sh`、
RUN状態遷移とcollector・digest・manifest処理は`measurectl run begin/finalize`が担当します。
`.config/wt.toml`の`pre-start`は`task worktree-link-raw`を呼び、新しいworktreeのルート`raw`を
primary worktreeへのディレクトリリンクにします。`runs/<RUN_ID>/raw`は各worktreeに保持します。
`raw`は`copy-ignored`の対象外です。リンク先が違う場合や既存の`raw`が空でない場合は作成を止めます。
並行改善は[isucon-worktree](.agents/skills/isucon-worktree/SKILL.md)に従い、app-server上の独立したCodexタスクと
専用worktreeへ実装を分担します。mainがタスクIDで指示・進捗・結果を確認し、順次統合・計測します。
Herdrのタブ操作は行わず、`post-merge`は`task q-sync`で分析DBを同期します。
`current-run-id`は1行目にRUN ID、2行目に開始元worktreeの正規化した絶対パスを記録します。
回収・profile操作・中断は開始元でのみ実行でき、不一致なら操作前に開始元を表示して停止します。
進行中RUNがある場合のデプロイ禁止は全worktreeに適用します。
開始・終了マーカーは`task bench-timestamp`でENTRY_HOSTのUTC時刻を取得し、ローカルPCの時計を
profileやアクセスログの時間窓へ混ぜません。取得失敗時はエラーとしてRUNをfinalizeします。
競技サーバー間の時計差はsetupで別途確認してください。

`after-bench`は回収・manifest確定後、そのRUNディレクトリ全体（`.gitignore`対象は除外）と
`runs/scores.tsv`を自動でローカルGitコミットします。失敗RUNも対象です。別RUN、
`raw/`、アプリ・設定変更は含めず、無関係なステージ済み変更も維持します。Gitコマンドは失敗時に3回まで再試行します（初回を含め最大4回）。自動コミットではGit hooksを実行しません。
対象ファイルが既にステージ済みの場合は、そのステージ内容を保護するためコミットを中止してエラーにします。
Gitコミットに失敗しても回収済み成果物は残ります（`git add`後の失敗では対象成果物がステージに残ります）。
RUNは確定済みなので`after-bench`を再実行せず、対象ファイルを確認して手動コミットしてください。pushは行いません。
`after-bench`はartifact検査後に分析DBも同期します。分析DBは再生成可能なローカル状態であり、Git commitの対象外です。

collector負荷は、同じ構成で通常RUNと次のRUNを取り、スコアとホストメトリクスを比較します。

```shell
task bench MEASURECTL_COLLECT_FLAGS=-no-collectors PROFILES_ENABLED=false
```

collectorなしRUNは`run.json.collectors_disabled=true`を記録し、周期collectorの成果物を任意扱いにします。
no-collectorsタスクではprofileも無効にしますが、nginx等のログ出力・回収は継続します。
access logやdigesterの必須成果物の欠損は引き続き検査失敗です。異なるcollectorモードのRUNは
通常の採用比較では互換とせず、計測負荷の比較として扱います。古いRUNの省略値は`false`として読みます。

250msのtask-state走査は既定では無効です。50msのMySQL lock wait取得は、各DBのlock waitを
同じRUNで比較できるよう`mysql_all`を対象に既定有効としています。利用を変更する場合は
`tools/measurectl/collectors.yaml`の`task-state`と`mysql-locks`の`enabled_by_default`を切り替えます。
設定変更後は通常の`task bench`で収集されます。

lock collectorの負荷とcapture errorはRUNごとに確認し、過大な場合はsampling intervalとquery timeoutを
見直します。各DBの成果物は`<host>-mysql-lock-waits.tsv`として保存されます。

主な成果物:

- `runs/<RUN_ID>/run.json` — source、役割、APPLIED snapshot、score、成果物状態、計測窓
- `alp.txt` / `alp.json` / `alp-by-ingress.tsv`
- `<host>-pt-query-digest.log` / `<host>-slp.tsv` / `<host>-mysql-digest.tsv`
- `<host>-proc-metrics.tsv` / service / disk / task-state（task-stateは高頻度collector明示時）
- `<host>-mysql-status.tsv` / `<host>-mysql-lock-waits.tsv`
- `<host>-sql-pool-metrics.tsv` — アプリのSQL接続プール（adapter導入時）
- `<host>-fgprof.pprof`（`task fgprof-collect`実行時）
- `<host>-go-cpu.pprof` / heap / allocs / goroutine（profile有効時）
- `raw/access-<host>.log.zst` — ホスト別nginxログ。空ログも圧縮して保存
- `<host>-app-journal.log` / `<host>-nginx-error.log`
- `<host>-kernel.log` / `<host>-oom.log`
- `upstream-breakdown*.tsv`
- `user-transitions.json` — setupで識別列と`tools/contest/user-transition-routes.json`を当日のAPIへ合わせる標準集計

nginxのJSON access logは、少なくとも`msec`、`method`、`uri`、`status`、`response_time`、`body_bytes`、
`upstream_time`、`upstream_addr`、`upstream_status`、`cache_status`を出してください。標準のユーザー遷移用に、個人情報や認証Cookieの生値を恒久保存せず、
セッションを結び付けられる識別子を専用フィールド（既定`session_id`）へ出します。集計成果物に識別値は残しません。

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
開始時の成果物宣言は`artifact_contract`へ固定します。有効なuser-transitionも必須で、識別情報やAPI分類がない場合は検査失敗となります。

`PROFILE_SECONDS`は初期化・整合性チェック・負荷時間・余裕を含めて設定します。既定の120秒は例です。
heap・allocs・goroutineは`SNAPSHOT_PROFILE_DELAY`後に同時取得します。既定の40秒も競技に合わせて変更します。
自動収集には標準pprofに加え、RUN所有情報を返す開始確認endpointが必要です。詳しくは導入例を参照してください。

```shell
task go-profile-top RUN=runs/<RUN_ID> PROFILE=isucon-1-go-cpu.pprof
```

`scores.tsv`の空欄はスコア不明、`0`は実際の0点です。スコアと計測成果物はRUN単位で保存し、比較時には役割・コード時点・計測窓・負荷条件を確認します。

## 分析

```shell
task alp
task q-build
task q -- "select run_id, score from runs order by score desc"
task dashboard
```

`task q`と`task dashboard`は分析DBを読み取るだけで、暗黙の同期は行いません。新しいRUNを
`after-bench`以外の方法で追加した場合や、分析schemaを変更した場合は、先に`task q-sync`を実行します。

dashboardの累積エラー件数は`bench_error_counts`から読み取り、負荷走行開始からの経過秒に対して報告時点の件数を表示します。個々のエラー発生時刻ではありません。
dashboardの時刻付き警告は分析DBの意味ビュー`bench_warning_events`から読み取ります。
起動後に追加したRUNは、`task q-sync`を実行してから`task q -- "SELECT * FROM bench_errors"`で確認します。
エラー内容の一覧はdashboardには表示せず、上記クエリで確認できます。
これら3つの意味ビューの本体は当日のベンチ出力の書式に依存するため、
`tools/contest/analysis-schema/bench-errors-semantic.sql`に置いています。
templateの状態ではプレースホルダーで0行を返すので、setupで当日の書式に合わせて書き換えます。

dashboardでは収集済みのfgprofに加え、GoのCPU・heap・allocs・goroutine profileを
種別・ホスト別に切り替え、関数ランキングとコールグラフで確認できます。コールグラフ表示にはGraphvizが必要です。

分析では、RUNごとの役割・source・計測窓を確認します。
CPU実仕事、I/O、lock/queue wait、DB query time、HTTP response timeを分け、変更境界が削減できる量を見積もります。

必要に応じて、保存済みaccess logの配置候補を`task topology-screen`で比較できます。
nginxのon-CPU profile、任意JSON endpointのsnapshot、ダッシュボードのブラウザ確認用CLIも
`tools/`に含まれています。導入やサーバー変更を伴うものは、対応する`task --list`のoptional taskを明示的に実行してください。

## Agent workflow

`.agents/skills/`では環境整備、対話による調査・実装、資料照合、資料作成を支援します。

- `isucon-setup` — WebUIのHARに基づくポータル対応、初期構築・正規deploy・ベンチ実行と計測不足の補修
- `isucon-special-sauce` — 設定資料を現行環境と照合し、適用できる改善を実装・正規deploy
- `isucon-use-solution` — 指定されたsolution文書の適用条件を現行環境と照合して報告
- `isucon-agent` — 必要な破壊的変更も含め、調査・実装・正規deploy・ベンチ・分析を自律的に繰り返す
- `isucon-create-solution` — 再利用できる実装パターンを`docs/solutions/`へ文書化

デプロイはTaskfileの正規経路を使い、対象ホストと影響を確認します。ベンチは`task bench`で実行し、結果と計測成果物を確認します。

## 再利用資料

- [`docs/measurement/`](docs/measurement/README.md) — アプリ・MySQL・nginxへの計測の組み込み
- [`docs/special-sources/`](docs/special-sources/README.md) — nginx、MySQL、systemd、sysctlの設定候補
- [`docs/solutions/`](docs/solutions/README.md) — N+1、index、bulk upsert、非同期化、in-memory、静的配信、PGO、UDSなど

これらは自動適用する完成設定ではありません。公式仕様、現行構成、計測値、正当性・評価条件を確認して採用します。

計測結果は`runs/`へ保存します。調査・実装の判断根拠は完了報告と必要な再利用資料へ記録し、作業用の集計ファイルは一時ディレクトリに置きます。

## Setup postflight

`task setup-postflight` checks the configured roles, connections, deployed
contents and collector cleanup.

<!--
# >>> contest values >>>
# <<< contest values <<<
-->
