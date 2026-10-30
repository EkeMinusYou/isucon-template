# ISUCON template — redesign

外部仕様と提供データから、HTTP/TLS・静的配信・業務処理・状態管理をGoで新規実装するテンプレートです。実行構成の正本は[AGENTS.md](AGENTS.md)です。nginx・DB・キャッシュサーバーは動かしません。

## 3段階

1. [isucon-setup](.agents/skills/isucon-setup/SKILL.md)で初期ソースをリポジトリ外の`REFERENCE_DIR`へ取得し、外部仕様を`docs/behavior.md`へ、提供データを独立したファイルへ整理します。
2. **別の新しいセッション**の[isucon-bootstrap](.agents/skills/isucon-bootstrap/SKILL.md)で、仕様・データ・資源条件から`webapp/go/`へ全機能を実装します。計測・配布・初期化・復元・初回PASSを確認します。
3. [isucon-agent](.agents/skills/isucon-agent/SKILL.md)で、得点につながる操作の進行と実測から改善します。

初期ソース・内部構成・setupの会話はbootstrapへ渡しません。`webapp/go/`の公開サーバー・静的配信・共通計測は再利用できます。業務API・認証方式・状態管理・永続化方式は固定しません。

## 設定と配布

`Taskfile.yml`のcontest values区画へ、ホスト・IP・service・port・初期化入力を設定します。

| 変数 | 用途 |
| --- | --- |
| `APP_HOSTS` | Goを動かすホスト |
| `INGRESS_HOSTS` | 公開HTTP入口とアクセスログの回収元。`APP_HOSTS`の範囲で指定 |
| `APP_TRAFFIC_HOSTS` | API処理を担当するGoホスト |
| `LISTEN_PORT` | HTTP待受。既定80 |
| `TLS_ADDR` | TLS待受。例`:443`。空なら無効 |
| `TLS_CERT_FILE`, `TLS_KEY_FILE` | ホスト上の証明書と鍵。TLS有効時に指定 |
| `STATIC_DIR`, `STATIC_REMOTE_DIR` | 不変の公開静的資材の配布元・配布先 |
| `STATIC_URL_PREFIX` | 静的配信のURL prefix。既定`/static/`。仕様に合わせる |
| `ACCESS_LOG_PATH` | GoのJSONログ。既定`/home/isucon/log/access.log` |
| `RESET_INPUTS` | 全初期化ファイル、不要なら`none` |

複数ホストへの外部トラフィックの振り分けと、状態の共有・所有権・必要なホスト間通信は競技側で設計します。`ENTRY_HOST`は`INGRESS_HOSTS`の先頭です。

```sh
task inspect-hosts
task setup             # Acquire original sources in the reference directory
# Prepare behavior and data, then start a new session

task setup-check       # Build, local validation and deployment plans
task deploy-all        # Apply OS, static assets, TLS and Go; stop unused services
task initialize
task setup-postflight
task bench
task compare-runs BASE_RUN=<RUN_ID> NEW_RUN=<RUN_ID>
```

`task gen`はGoのsystemd unitを生成します。低位portのbind capabilityと`LimitNOFILE=65536`を設定し、計測listenerは`127.0.0.1:6060`へ分離します。`APP_ENV`は非機密の追加環境変数mapです。

TLS資材はGit対象外の`tls/`へ置くと、`deploy-all`で`TLS_REMOTE_DIR`（既定`/home/isucon/tls`）へ配布します。`TLS_ADDR=:443`と、配布した証明書・鍵のパスをTaskfileへ指定します。TLSではHTTP/2を有効にし、ホスト名・証明書の信頼点・有効期間を公開検証で確認します。秘密情報はcommitしません。

静的資材は`public/`からGoホストへ配布します。既定は1日キャッシュ、ETag/Last-Modified、静的MIMEのみのgzip、範囲要求、上限付きのファイル内容キャッシュです。[秘伝のタレの対応](docs/special-sources/nginx.md)を参照してください。動的画像・認証付き応答・APIはこの配信へ混ぜず、更新と初期化の可視性に応じた実装を用意します。

`task deploy`はGoのbuild・配布・restartを行い、初期化しません。静的資材だけの更新は`task deploy-static`です。永続データは配布時の削除対象外に置きます。追加の配布は`tools/contest/`の設定から共通graphへ接続します。

`deploy-app-dry`・`deploy-static-dry`・`deploy-all-dry`・`initialize-dry`は接続せず配布計画を表示します。サーバー変更とベンチは操作ロックで直列化します。同じサーバーを使う別リポジトリのRUNとも重ねません。

## 計測と解析

Goが公開接続からJSONアクセスログを出します。共通ロガーは5msごとのまとめ書き・100msごとのflushで、出力失敗とqueue残量をruntime指標へ公開します。認証後に`setMeasurementIdentity(r, identity)`を呼ぶと、秘密情報を直接出さずに操作を結び付けられます。Cookie名は仮定しません。

`task bench`は開始前に全入口のログをtruncateし、終了後2秒待ってSSH/rsyncで回収します。生ログをRUNごとにzstd圧縮保存し、alp・操作遷移・DuckDBで共用します。alpはRUN全体、DuckDBは要求数から推定した負荷期間を対象にします。[ログ契約と解析例](docs/measurement/go-access-log.md)を参照してください。

接続ID・HTTPバージョン・接続内の要求数・TLS再利用を記録します。Linuxでは初回と32要求ごとにTCPのRTT・再送累計を採取し、未対応環境では省略します。別ノードへのHTTP通信には共通の`measurement.NewUpstreamTransport()`を使えます。入力要求のcontextを引き継ぐと、接続確立・TLS・ヘッダー待ち・本文読取の観測を同じ要求へ記録します。

```sh
task alp
task q -- "select * from load_windows"
task q -- "select * from connection_reuse"
task q -- "select * from tcp_samples limit 20"
task q -- "select * from upstream_requests limit 20"
task q -- "select * from access_log_health"
task artifacts-run RUN=runs/<RUN_ID>
```

[Go profile](docs/measurement/go-profiling.md)、[状態・操作](docs/measurement/state-and-operations.md)、ホスト・サービス指標とjournalも共通の負荷区間で解析します。HTTP/2の並行streamをHTTP/1の逐次接続の待ちとして扱いません。欠測はNULLやartifact statusで区別します。

API分類と操作定義は`tools/contest/user-transition-routes.json`へまとめ、alp・操作遷移・DuckDBで共有します。公開HTTP応答・競技固有テストは`tools/contest/validation.json`へ宣言して`task validate`で確認します。

Go土台は`/healthz`だけを実装しています。`/initialize`・業務API・認証・データの復元はbootstrapで実装します。`task test-app`と`task test-measurement`で公開入口と計測を検証できます。必要なローカルツールはGo、go-task、SSH、rsync、alp、zstd、DuckDBです。dashboardを使う場合だけNode.jsも必要です。
