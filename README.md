# ISUCON template — redesign

外部の振る舞いを維持し、nginxを入口に、業務処理と状態管理をGoでゼロから作るテンプレートです。既存方式との比較用に`isucon-template`から分けて管理します。実行構成の制約は[AGENTS.md](AGENTS.md#全面再設計の方針)を正本とします。

HTTP/TLS・静的配信・圧縮・リバースプロキシをnginx、業務処理・状態管理・索引・検索・永続化・復元をGoが担います。DB・キャッシュサーバー等は使用しません。OS・systemd、開発・配布・ベンチ・分析ツール、および公式仕様が要求する外部サービスは利用します。

## 3段階

1. [isucon-setup](.agents/skills/isucon-setup/SKILL.md)で初期ソースと公式資料を確認し、外部仕様を`docs/behavior.md`へ整理します。旧ソースはリポジトリ外の`REFERENCE_DIR`へ取得します。初期データはGoから直接読めるファイルへ分離します。
2. **別の新しいセッション**の[isucon-bootstrap](.agents/skills/isucon-bootstrap/SKILL.md)で外部仕様・提供データ・資源条件から設計し、汎用土台のある`webapp/go/`へ業務ロジックを新規実装します。計測・配布・初期化・復元を整え、公式ベンチの初回PASSまで確認します。
3. [isucon-agent](.agents/skills/isucon-agent/SKILL.md)で、得点への効果から仮説を選び、実装・計測・比較を続けます。

setupの会話・初期ソース・内部構成はbootstrapへ渡しません。bootstrapからagentへは同じセッションで進めます。

## 設定と操作

構成の正本は`Taskfile.yml`です。ホスト・IP・Goサービス・port・TARGET_OS/ARCH・初期化資材を当日の環境へ合わせます。`APP_HOSTS`はGoプロセスを置くホスト、`APP_TRAFFIC_HOSTS`はnginxの転送先、`NGINX_HOSTS`は公開HTTP入口です。既定は1台にnginxとGoを置き、Goは8080番で待ち受けます。`DISABLED_SERVICES`には停止・無効化する不要なミドルウェアのunitを列挙します。setupは旧サービスの調査を先に済ませ、bootstrapでの切り替え時に停止します。

ローカルにはGo、go-task、SSH、rsync、alp、zstd、DuckDBが必要です。dashboardを使う場合だけNode.jsを使います。競技サーバーへはGoバイナリをクロスコンパイルして配布します。

```sh
task inspect-hosts     # 使用可能な資源の確認
task setup             # 汎用Go土台を維持し、旧ソースを調査用領域へ取得
# ここで外部仕様・データを準備し、別セッションへ切り替える

task setup-check       # 新実装のbuild・検査・配布計画を確認
task deploy-all        # 不要サービス停止、OS設定・Goアプリ・nginx配布
task initialize        # Goアプリの初期化APIを一度実行
task setup-postflight  # 配布物・役割・通信・collector残存を確認
task bench             # ベンチ実行と計測・回収・分析
task compare-runs BASE_RUN=<基準RUN> NEW_RUN=<新RUN>
```

`task deploy`はGoアプリのbuild・配布・restartを行います。初期化はしません。通常再起動で受理済み更新を保持できるよう、永続データは配布ディレクトリの外へ置きます。`RESET_INPUTS`には初期化に必要なファイルを列挙し、不要なら`none`を指定します。追加の生成・配布は`prepare-initial-data`と`tools/contest/`の配布設定へ接続します。

`APP_ENV`に非機密の環境変数mapを指定すると、`task gen`がGoサービスのsystemd unitへ反映します。
既定は空で、GC・メモリ上限等の値は固定しません。待受・計測アドレスを変える場合はTaskfileのport・collector設定も揃えます。
Goアプリの配布・unitの起動は、既定で各APP_HOSTSの`/healthz`が200を返すまで最大30秒待ちます。
判定URL・期限・期待statusは[配布宣言のready](tools/README.md#デプロイ)で変更できます。bootstrapは復元を完了してから準備完了を返すよう実装し、復元するデータの正当性は競技側のテストで確認します。

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

競技サーバーの初期nginx設定は、他の初期ソースと同様にリポジトリへ含めません。`task setup-nginx`は`REFERENCE_DIR`へ取得し、setupは外部仕様だけを渡します。テンプレートには[秘伝のタレ](docs/special-sources/nginx.md)に基づく汎用ベースを同梱し、bootstrapでTLS・URL・静的資材など競技固有の条件を設定します。

## 競技仕様と共通の検査

API分類は`tools/contest/user-transition-routes.json`にまとめ、alp・操作遷移・DuckDBで共用します。
`task check-api-routes`で宣言を検査し、RUN開始時の定義を保存します。ベンチ回収後と`task compare-runs`では未分類・定義欠損と集計件数の対応も自動表示します。

公開HTTP応答の期待値と競技固有のテストは`tools/contest/validation.json`へ宣言し、`task validate`で実行・保存します。
`task setup-postflight`にも接続済みです。通知方式、必要な観測順序、URL、キャッシュ期限は競技側で決めます。
空の宣言は未設定として扱い、既定の業務シナリオは持ちません。[設定と結果の扱い](tools/validation/README.md)を参照してください。

## 業務ロジックを持たないGo土台

`webapp/go/`は起動・終了処理と共通計測moduleへの接続だけを持ちます。API・認証・状態管理・永続化はbootstrapで実装します。既定で`/healthz`だけを提供し、`/initialize`や競技APIは未実装の404です。

nginxが公開HTTP/TLSを受け、内部HTTPでGoへ転送します。Goの`APP_ADDR`は既定`:8080`、計測用の`MEASUREMENT_ADDR`は`127.0.0.1:6060`です。計測は別listenerに分離し、公開routerには登録しません。Goの起動方法や待受を変更しても、共通計測moduleとcollectorのURLを対応させれば構いません。

`server.go`の`serverConfig.Snapshot`に内部指標を追加できます。`measurement.go`の`setMeasurementHeaders`は、bootstrapが選んだ認証セッションと実際のbody読取量をnginxログへ渡す接続口です。Cookie名や認証方式は仮定しません。標準アクセスログはnginxが出し、Goでは重複出力しません。

`task gen`はsystemd unitも`generated/systemd/<SERVICE>.service`へ生成し、`APP_NAME`・`APP_DIR`・`REMOTE_HOME`・`ISUCON_USER`・`LISTEN_PORT`を反映します。`task deploy`でこのunitとGoバイナリを配布します。`task test-app`で起動・計測分離・終了を確認できます。初期化・復元・公式ベンチPASSは業務実装後にbootstrapで検証します。

## 複数台のnginx設定

- `nginx/common/`：全ホスト共通の基本設定・ログ・圧縮・接続設定。
- `nginx/roles/<role>/`：役割ごとの設定。共通設定へ重ね、同じ相対パスのファイルは役割側を採用します。既定は`edge`です。
- `Taskfile.yml`：`NGINX_HOSTS`、IP、Goの配置と転送先を定義します。
- `generated/nginx/<host>/`：`task gen`が作る配布物。直接編集せず、各ホストの`/etc/nginx/`へ配布します。

最初は1台で使えます。`NGINX_HOSTS`へホストを追加すると、同じ`edge`設定と`APP_TRAFFIC_HOSTS`の転送先を使います。ホストごとの違いが必要になった場合だけ、`NGINX_HOST_CONFIG`を指定します。

```yaml
  NGINX_HOSTS: isucon-1 isucon-2
  APP_HOSTS: isucon-1 isucon-2 isucon-3
  APP_TRAFFIC_HOSTS: isucon-1 isucon-2 isucon-3
  NGINX_HOST_CONFIG:
    map:
      isucon-2:
        role: edge
        app_hosts: [isucon-2, isucon-3]
```

外部からの入口は引き続き`ENTRY_HOST`（`NGINX_HOSTS`の先頭）です。複数台への外部トラフィックの振り分けは、競技の接続条件に合わせて別途設計します。

この例ではisucon-1が既定の全転送先、isucon-2が指定した2台へ転送します。別の役割が必要なら`nginx/roles/<role>/`を追加して指定します。転送先は`APP_HOSTS`の範囲で選びます。`listen_port`でホスト別のHTTP待受portも指定でき、省略時は`NGINX_PORT`を使います。TLSを使うroleでは`@@TLS_PORT@@`を参照でき、既定は`NGINX_TLS_PORT`（443）、ホスト別の`tls_port`で上書きします。この値の指定だけではTLSを有効にせず、listenerと証明書はrole側で構成します。`task check-network`も各ホストが実際に使う転送先を検査します。役割から外したホストの生成物は次の`task gen`で除去され、サービス停止は`task apply-roles`または`task deploy-all`が行います。

`task gen-contest-routing`は共通生成後の競技固有hookです。通常のホスト別設定には上記の入力を使い、hookで生成物を手修正しません。nginx設定は`task deploy-nginx-dry`で配布先を確認し、`task deploy-nginx`で構文検査・反映します。生成失敗時は以前の生成物を保持し、配布へ進みません。

公開静的資材は`STATIC_DIR`（既定`public/`）から全NGINX_HOSTSの`STATIC_REMOTE_DIR`へ配布します。役割のserver設定で仕様を確認したURLだけを直接配信し、必要なlocationへ`static-cache.conf`をincludeします。動的アップロードや永続データは、配布時に削除されるこのディレクトリへ置かず、別の保存・同期経路を設計してください。TLS秘密鍵は取得・生成物へ含めず、配布先に保持します。OS提供の`modules-enabled/`と`mime.types`も配布時の削除対象にしません。

`task test-nginx`で生成処理を検証できます。`NGINX_TEST_IMAGE=nginx:1.26.3 task test-nginx`ではDocker内で2台分の生成設定・proxy・キャッシュ・圧縮・計測ログも確認します。
