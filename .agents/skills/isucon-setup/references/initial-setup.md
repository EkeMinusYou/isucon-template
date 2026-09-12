# 初回セットアップ

初期環境を構築するときに読む。[isucon-setup](../SKILL.md)の作業境界・完了条件と、[標準計測の確認手順](measurement.md)を併用する。

## スキーマ再作成を含むデプロイ

初回setupの作成対象として、`Taskfile.yml`に`deploy-all-reset`ターゲットを必ず用意する。
既存ターゲットがあれば、公式の初期化仕様と実配置に合わせて補修する。

- MySQL等のアプリ用DBをスキーマから破棄・再作成し、全体のデプロイ、初期データ投入、公式仕様で必要な初期化、疎通確認までを一つのターゲットで実行できるようにする。
- 対象ホスト・DB・schemaはTaskfileの役割定義と取得済み資産から決める。共有DBは正本ホストで一度だけ再作成し、他用途のDBを一律に破棄しない。
- build等のローカル準備を先に済ませ、DB再作成前に書込み元のアプリを停止する。スキーマが必要なmigrationやアプリ起動より先に再作成を完了させ、途中で失敗したら後続処理へ進まない。
- 既存の`task deploy-*`、deploy宣言、公式の初期化処理を再利用する。競技データの保持を前提とする移行・バックアップ・逆移行スクリプトは、このフローのために追加しない。
- 通常の`deploy-all`はデータを保持する。`deploy-all-reset`にも進行中RUNを保護する既存の制約を適用し、ベンチマーカーは組み込まない。
- READMEへ実行方法と破棄・再作成の対象を記載し、dry-run等で実行順序・対象を検証する。ターゲットの作成と実機での実行は区別し、実行はユーザーの依頼範囲に従う。

## 標準計測

以下は初回setupの必須対象である。未実装・未設定を理由にoptionalへ落とさない。

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
4. `task deploy-all-reset`を上記の要件で作成・検証し、全管理対象を正規deploy・role収束・計測・分析・RUNの役割記録へ組み込む。標準計測のadapterと通常ベンチの起動・回収待ちも整える。
5. 対応する`task deploy-*-dry`で転送先とactivationを確認して正規deployし、`task check-roles`・`task check-network`と計測の疎通を確認する。
6. `task artifacts`と短いログ・profile取得で生成から読み手までを確認する。collectorの起動・停止・timeout・欠損検出も確認する。
7. ユーザーが実行したbaseline RUNで`task artifacts-run`、内容・ホスト・時間窓、DuckDB・dashboard表示を確認する。
   通常の回収は失敗RUNも`after-bench`でfinalizeする。自動ローカルcommitの副作用を確認し、`abort-run`を通常回収の代用にしない。
8. 初期Objectiveの整備はisucon-objectiveの担当として案内し、確認した公式採点仕様とbaseline RUNの参照を完了報告へ残す。
