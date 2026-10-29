# ISUCON template — redesign

外部の振る舞いを維持し、Goのみで公開HTTP/TLSから業務処理・状態管理まで新規設計するテンプレートです。実行構成の制約は[AGENTS.md](AGENTS.md#全面再設計の方針)を正本とします。nginx・DB・キャッシュサーバー等は起動せず、OS・systemd・開発/計測ツールと公式の外部サービスを利用します。

## 3段階

1. [setup](.agents/skills/isucon-setup/SKILL.md)で初期ソースと公式資料から外部仕様を`docs/behavior.md`へ整理します。調査用ソース・初期nginx設定はリポジトリ外の`REFERENCE_DIR`へ取得し、初期データを独立したファイルへ分離します。
2. **別の新しいセッション**の[bootstrap](.agents/skills/isucon-bootstrap/SKILL.md)で外部仕様・データ・資源から設計します。[Go HTTPの秘伝のタレ](docs/special-sources/go-http.md)と`webapp/go/`の土台へ業務実装を加え、配布・計測・復元・公式ベンチの初回PASSまで確認します。
3. [agent](.agents/skills/isucon-agent/SKILL.md)で得点への効果から仮説を選び、実装・計測・比較を続けます。

setupの会話・初期ソース・内部構成はbootstrapへ渡しません。業務API・認証・データ構造・永続化はテンプレートに含めません。

## 設定と操作

`Taskfile.yml`が構成の正本です。`APP_HOSTS`がGoの配置先、`APP_TRAFFIC_HOSTS`が公開入口で、先頭が`ENTRY_HOST`です。既定は1台のGoが8080番を直接待ち受けます。TLSが必要なら`LISTEN_PORT`とサーバー上の`TLS_CERT_FILE`・`TLS_KEY_FILE`を指定します。nginxは`DISABLED_SERVICES`に含まれます。

```sh
task inspect-hosts
task setup             # 調査用ソース・設定をリポジトリ外へ取得
# 外部仕様・データを準備して別セッションへ切り替える

task gen               # HTTP設定・事前圧縮資材・systemd unitを生成
task setup-check
task deploy-all        # 不要サービス停止、OS・Go・HTTP設定・静的資材を配布
task initialize        # Goの初期化APIを一度実行
task setup-postflight
task bench
task compare-runs BASE_RUN=<基準RUN> NEW_RUN=<新RUN>
```

`task deploy`はbuild・配布・restartを行い、初期化しません。`RESET_INPUTS`に初期化に必要な全ファイル、不要なら`none`を指定します。永続データは配布ディレクトリの外へ置きます。競技固有の生成・配布は`Taskfile.contest.yml`・`tools/contest/`へ置きます。

配布前に`task deploy-app-dry`・`task deploy-all-dry`で対象と実行順を確認できます。サーバー変更とベンチは共通の操作ロックで直列化します。別リポジトリの同じ競技環境と並行実行しません。ポータル設定は`tools/contest/`と`.env.local`に置き、秘密情報はcommitしません。

## Go HTTPの初期実装

`webapp/go/`には公開HTTP/TLS・HTTP/2、keepalive、接続上限、明示した不変資材のキャッシュ・事前圧縮配信、SSE/WebSocketを妨げないアクセスログ、起動・終了を用意しています。業務routerは`routes.go`へ実装します。既定は`/healthz`のみで、`/initialize`や業務APIは404です。

`HTTP_STATIC_ROUTES`で公開URL・ディレクトリ・キャッシュ方針を指定します。未設定なら静的資材は公開しません。`task gen`は`public/`から`generated/public/`へ元ファイルと小さくなるgzip版を生成します。配布先は`/home/isucon/public`です。登録した資材は起動時にメモリへ読み込み、元ファイルとgzip版の一致を検証します。更新データ・認可付きファイル・動的アップロードは別handlerと永続領域で扱います。

```yaml
  HTTP_STATIC_ROUTES: '[{"prefix":"/assets/","directory":"/home/isucon/public/assets","cache_control":"public, max-age=86400"}]'
  HTTP_HOST_CONFIG:
    map:
      isucon-2:
        max_connections: 16384
```

`HTTP_HOST_CONFIG`で`APP_HOSTS`ごとの待受、TLSパス、接続上限、静的routesを上書きできます。生成物は`generated/http/<host>.json`、配布先は`/home/isucon/http.json`です。秘密鍵は参照パスだけを設定し、生成物やGitに含めません。Go間転送が必要なら`newUpstreamTransport()`を使い、ルーティング・更新順序・再試行を仕様に合わせて実装します。分散を初期構成として強制しません。

## 計測と検証

公開トラフィックと計測は別listenerです。`MEASUREMENT_ADDR`はloopbackの6060番、公開routerは`/debug`以下を公開しません。`serverConfig.Snapshot`へ業務指標を追加できます。

アクセスログはGoが`ACCESS_LOG_PATH`（既定`/home/isucon/access.log`）へJSON出力します。認証後に`setRequestSession(r, identity)`を呼ぶと、ハッシュ化した識別値で操作間を対応付けます。生tokenや計測ヘッダーをクライアントへ返しません。body読取量・応答サイズ・接続再利用・TLS再利用とloggerの健全性は土台で接続済みです。

`task bench`がRUN採番、計測開始・停止・回収、alp・DuckDB・操作遷移の集計とローカルcommitを行います。`run.json`の合否・得点・成果物状態を確認し、欠損を0と扱いません。既存のdashboardはGoの入口ログとruntime指標を表示します。

```sh
task test-app          # TLS/HTTP2・静的配信・304・Range・SSE・計測分離
task test-http         # 静的資材の生成・再生成
task q -- "select * from go_runtime_window"
task q -- "select * from access_log_health"
task artifacts-run RUN=runs/<RUN_ID>
```

ローカルにはGo、go-task、Python、SSH、rsync、alp、zstd、DuckDBが必要です。dashboardにはNode.jsを使います。Goは対象OS/ARCHへクロスコンパイルして配布します。詳細は[Tools](tools/README.md)、[計測](docs/measurement/README.md)、[Evidence](.agents/skills/_shared/evidence.md)を参照してください。
