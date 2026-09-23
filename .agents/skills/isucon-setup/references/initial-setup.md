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

- スコアの記録方法（生成元、取得・入力手順、RUNへの紐付け、保存形式）と、成功・失敗判定、公式出力にある内訳・ペナルティの保存・分析・表示。必要に応じたDuckDBの取り込みschema・意味ビューの対応。
- Go CPU・heap・allocs・goroutineのpprof、fgprof、採取開始確認endpoint。
- nginxアクセスログの必要列、ローテート、全NGINX_HOSTSの回収、alp・upstream集計。
- user-transitionの識別方法・ログ列・アプリ固有API分類・集計。
- DB・ホスト・全管理対象serviceと依存サービスの標準メトリクス、app/nginx・kernel/OOM等の障害ログ。
- 通常ベンチへの自動収集接続、ホスト別の必須成果物・内容・時間窓の検査、対応するDuckDB・dashboardの読み手。

有効宣言と対象外判断は[標準計測の確認手順](measurement.md#有効宣言と対象外判断)に従う。

## 初回セットアップ

1. 公式資料から構成・変更可能範囲・初期化・採点・追試条件を確認する。
2. 読み取り専用SSHで全ホストの資源、IP、service・unit・process・listen portと依存経路を確認する。
   管理対象のアプリ・設定・unit・schemaを`task setup-*`で取得し、対象外は理由を残す。
3. Taskfileの役割・IP・service名・TARGET_OS/ARCH・schema取得先・構文検査を実環境へ合わせ、`task gen`と`task setup-check`を通す。
4. `task db-recreate`を上記の要件で作成・dry-run確認し、全管理対象を正規deploy・role収束・計測・分析・RUNの役割記録へ組み込む。標準計測のadapterと通常ベンチの起動・回収待ちも整える。
5. 対応する`task deploy-*-dry`で転送先とactivationを確認して正規deployし、`task check-roles`・`task check-network`と計測の疎通を確認する。
6. `task artifacts`と短いログ・profile取得で生成から読み手までを確認する。collectorの起動・停止・timeout・欠損検出も確認する。
7. ユーザーが実行したbaseline RUNで`task artifacts-run`、内容・ホスト・時間窓、DuckDB・dashboard表示を確認する。
   通常の回収は失敗RUNも`after-bench`でfinalizeする。自動ローカルcommitの副作用を確認し、`abort-run`を通常回収の代用にしない。
8. 確認した公式採点仕様とbaseline RUNの参照を完了報告へ残す。
