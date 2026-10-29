# ISUCON template — redesign

外部の振る舞いを維持し、nginxを入口に、業務処理と状態管理をGoでゼロから作るテンプレートです。既存方式との比較用に`isucon-template`から分けて管理します。実行構成の制約は[AGENTS.md](AGENTS.md#全面再設計の方針)を正本とします。

HTTP/TLS・静的配信・圧縮・リバースプロキシをnginx、業務処理・状態管理・索引・検索・永続化・復元をGoが担います。DB・キャッシュサーバー等は使用しません。OS・systemd、開発・配布・ベンチ・分析ツール、および公式仕様が要求する外部サービスは利用します。

## 3段階

1. [isucon-setup](.agents/skills/isucon-setup/SKILL.md)で初期ソースと公式資料を確認し、外部仕様を`docs/behavior.md`へ整理します。旧ソースはリポジトリ外の`REFERENCE_DIR`へ取得します。初期データはGoから直接読めるファイルへ分離します。
2. **別の新しいセッション**の[isucon-bootstrap](.agents/skills/isucon-bootstrap/SKILL.md)で外部仕様・提供データ・資源条件から設計し、空の`webapp/go/`へ実装します。計測・配布・初期化・復元を整え、公式ベンチの初回PASSまで確認します。
3. [isucon-agent](.agents/skills/isucon-agent/SKILL.md)で、得点への効果から仮説を選び、実装・計測・比較を続けます。

setupの会話・初期ソース・内部構成はbootstrapへ渡しません。bootstrapからagentへは同じセッションで進めます。

## 設定と操作

構成の正本は`Taskfile.yml`です。ホスト・IP・Goサービス・port・TARGET_OS/ARCH・初期化資材を当日の環境へ合わせます。`APP_HOSTS`はGoプロセスを置くホスト、`APP_TRAFFIC_HOSTS`はnginxの転送先、`NGINX_HOSTS`は公開HTTP入口です。既定は1台にnginxとGoを置き、Goは8080番で待ち受けます。`DISABLED_SERVICES`には停止・無効化する不要なミドルウェアのunitを列挙します。setupは旧サービスの調査を先に済ませ、bootstrapでの切り替え時に停止します。

ローカルにはGo、go-task、SSH、rsync、alp、zstd、DuckDBが必要です。dashboardを使う場合だけNode.jsを使います。競技サーバーへはGoバイナリをクロスコンパイルして配布します。

```sh
task inspect-hosts     # 使用可能な資源の確認
task setup             # 調査用ソースと空の実装先を準備
# ここで外部仕様・データを準備し、別セッションへ切り替える

task setup-check       # 新実装のbuild・検査・配布計画を確認
task deploy-all        # 不要サービス停止、OS設定・Goアプリ・nginx配布
task initialize        # Goアプリの初期化APIを一度実行
task setup-postflight  # 配布物・役割・通信・collector残存を確認
task bench             # ベンチ実行と計測・回収・分析
task compare-runs BASE_RUN=<基準RUN> NEW_RUN=<新RUN>
```

`task deploy`はGoアプリのbuild・配布・restartを行います。初期化はしません。通常再起動で受理済み更新を保持できるよう、永続データは配布ディレクトリの外へ置きます。`RESET_INPUTS`には初期化に必要なファイルを列挙し、不要なら`none`を指定します。追加の生成・配布は`prepare-initial-data`と`tools/contest/`の配布設定へ接続します。

`task config-check`は`CONFIG_CHECK_COMMAND`でローカル検査します。既定はGoのvetです。配布設定の確認には`deploy-app-dry`・`deploy-all-dry`・`initialize-dry`を使い、実サーバーへ接続せず計画を表示します。

サーバー変更とベンチは共通の操作ロックで直列化します。別リポジトリとはロックを共有しないため、同じ競技環境の別試行と同時に実行しません。ポータル設定と認証変数は`tools/contest/`と`.env.local`で設定し、秘密情報はcommitしません。

## 計測と分析

標準計測は[Go profile](docs/measurement/go-profiling.md)、[nginxアクセスログ](docs/measurement/nginx-access-log.md)、[状態・操作の計測](docs/measurement/state-and-operations.md)、ホスト・nginx・Goサービスの資源、障害ログです。永続化方式にかかわらず、得点につながる操作の件数・進行・待ち時間を確認できるようにします。

`task bench`がRUNを採番し、計測開始・ベンチ実行・停止・回収・集計・ローカルcommitを行います。失敗RUNも保存します。`runs/<RUN_ID>/run.json`の合否・得点・成果物状態を確認し、欠損を0と扱いません。負荷期間だけの分析値はDuckDBへ、期間を分離できない集計はRUNの成果物へ残します。

```sh
task q -- "select * from load_windows"
task q -- "select * from go_runtime_window"
task artifacts
task artifacts-run RUN=runs/<RUN_ID>
```

詳細は[Tools](tools/README.md)、判断基準は[Evidence](.agents/skills/_shared/evidence.md)を参照してください。

nginxの初期設定は、他の初期ソースと同様にリポジトリへ含めません。`task setup-nginx`は`REFERENCE_DIR`へ取得し、setupは外部仕様だけを渡します。bootstrapが[nginxの秘伝のタレ](docs/special-sources/nginx.md)を参照して設定を新規作成し、初回ベンチ前に適用します。`task gen`は新構成の転送先と静的配信rootを生成し、`task deploy-nginx`は構文確認付きの配布・reloadを行います。

公開静的資材は`STATIC_DIR`（既定`public/`）から全NGINX_HOSTSの`STATIC_REMOTE_DIR`へ`task deploy-nginx`で配布します。新規作成する`nginx/sites-enabled/app.conf`で仕様を確認したURLだけを`try_files`等で直接配信し、キャッシュ設定を加えます。動的アップロードや永続データは、配布時に削除されるこのディレクトリへ置かず、別の保存・同期経路を設計してください。

新規作成した設定は、`task deploy-nginx-dry`で配布先を確認し、nginxのバージョンとTLS・公開portを合わせてから反映してください。
