# 初回セットアップ

初期環境を構築するときに読む。[isucon-setup](../SKILL.md)の作業境界・完了条件と、[標準計測の確認手順](measurement.md)を併用する。

## DB再作成と通常デプロイ

公式資料とアプリの初期化処理を確認し、DBを新規構築する手順と再初期化する手順を整理する。

- 通常の`task deploy-all`はDBを再作成しない。`task db-recreate`は書込み元を止め、DBを正規手順で作り直し、アプリ初期化後に書込み元を戻す。
- 対象ホスト・DB・schemaはTaskfileの役割定義と取得済み資産から決める。共有DBは正本ホストで一度だけ再作成する。
- 主schema以外にDB構築や初期化が読むローカル資材は`RESET_INPUTS`へ列挙し、deploy設定のuploadと実行順も揃える。不要と確認した場合だけ`none`にする。
- build等の準備と`task setup-preflight`を先に済ませる。DB再作成前に書込み元を停止し、途中で失敗したら後続処理へ進まない。
- READMEに破棄対象と実行方法を記載し、dry-runで対象と順序を確認する。実DBの状態も読み取り専用で確認する。

## 標準計測

以下は初回setupの必須対象である。未実装・未設定を理由にoptionalへ落とさない。

- スコアの記録方法（生成元、取得・入力手順、RUNへの紐付け、保存形式）と、成功・失敗判定、公式出力にある内訳・ペナルティの保存・分析。必要に応じたDuckDBの取り込みschema・意味ビューの対応。
- 参加者向けベンチ出力が負荷中に定期的に出す進捗行（負荷の増減とその理由など）の時系列解釈ビュー。書式は当日のベンチ出力で確定するため、[ベンチ後の計測確認](#ベンチ後の計測確認)で作成する。
- Go CPU・heap・allocs・goroutineのpprof、fgprof、採取開始確認endpoint。
- nginxアクセスログの必要列、ローテート、全NGINX_HOSTSの回収、alp・upstream集計。
- user-transitionの識別方法・ログ列・アプリ固有API分類・集計。
- DB・ホスト・全管理対象serviceと依存サービスの標準メトリクス、app/nginx・kernel/OOM等の障害ログ。
- 通常ベンチへの自動収集接続、ホスト別の必須成果物・内容・時間窓の検査、対応するDuckDB・dashboardのAPI・parser。

有効宣言と対象外判断は[標準計測の確認手順](measurement.md#有効宣言と対象外判断)に従う。

## 初回セットアップ

1. 公式資料から構成・変更可能範囲・初期化・採点・追試条件を確認する。
2. 読み取り専用SSHで全ホストの資源、IP、service・unit・process・listen portと依存経路を確認する。
   管理対象のアプリ・設定・unit・schemaを`task setup-*`で取得し、対象外は理由を残す。
3. Taskfileの役割・IP・service名・TARGET_OS/ARCH・schema取得先・構文検査を実環境へ合わせる。
4. `task db-recreate`を上記の要件で整備し、全管理対象を正規deploy・role収束・計測・分析・RUNの役割記録へ組み込む。Go計測は`tools/measurement`の共通moduleを接続し、DB pool・service・接続先と競技固有の指標だけを設定する。標準計測のadapterと通常ベンチの起動・回収待ちも揃える。
5. 必要な修正を揃えてから`task setup-check`を通す。生成、DB再作成・deployのdry-run、build・テスト・成果物契約の確認はTaskfileの依存タスクに従い、含まれている検査は重ねて実行しない。転送先とactivationを確認して正規deployする。
6. `task check-roles`・`task check-network`と短いログ・profile取得で疎通を確認する。同じsmoke成果物で生成からDuckDB・dashboardのAPI・parserまでを確認し、collectorの起動・停止・timeout・欠損検出も確認する。
7. 確認した公式採点仕様、標準計測全件の状態・根拠、DB再作成のdry-runと実機実行の有無・結果を報告して初回setupを完了する。実負荷の確認はベンチ待ちとする。

確認後の再実行は、追加変更・失敗・未解決の懸念がある範囲に絞る。

## ベンチ後の計測確認

ユーザーがベンチを実行し、計測確認を依頼したときに行う。

1. 対象RUNで`task artifacts-run`、内容・ホスト・時間窓、DuckDB・dashboardのAPI・parserでの分析結果を確認する。
   通常の回収は失敗RUNも`after-bench`でfinalizeする。自動ローカルcommitの副作用を確認し、`abort-run`を通常回収の代用にしない。
2. 進捗行の解釈ビューが未作成なら、対象RUNの`bench.log`から負荷中に繰り返し出る進捗行を特定し、負荷開始からの経過秒・指標名・値（累計・割合・単発の区別付き）を返すビューを`tools/contest/analysis-schema/`に作って`sources.yaml`へ登録する。運営向け出力やベンチマーカー内部の情報は使わず、保存済みRUNで負荷中の推移が読めることを確認する。
3. 対象RUNの参照と実負荷で確認した項目・未検証範囲を報告する。
