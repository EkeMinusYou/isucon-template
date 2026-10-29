# Go HTTP：nginxの秘伝のタレに相当する初期設定

Goのみで公開HTTP/TLS、静的配信、接続再利用、計測を担います。`webapp/go/`が実装の土台、`Taskfile.yml`が配置と設定の正本です。業務API・認証・状態管理・永続化はbootstrapで実装します。

| 目的 | Goの初期実装・設定 |
| --- | --- |
| 接続・FD容量 | 1プロセスの受入上限`max_connections=32768`、systemdの`LimitNOFILE=65536` |
| keepalive | `net/http`の接続再利用、`IdleTimeout=120s`。要求数による強制切断は追加しない |
| 遅いヘッダー受信 | `ReadHeaderTimeout=5s`。body・応答全体の固定timeoutは置かず、仕様に応じて経路別に決める |
| TLS・HTTP/2 | 証明書と秘密鍵を指定すると`ServeTLS`で公開。TLS 1.2以上、標準のHTTP/2とsession ticketを利用 |
| upstreamの再利用 | `newUpstreamTransport()`でidle全体4096、宛先あたり1024、idle timeout 90s。必要なGo間転送・外部サービスへ適用 |
| 静的ファイルの圧縮 | `task gen`で元ファイルと小さくなるgzip版を生成し、`Accept-Encoding`に応じて配信 |
| 静的ファイルのキャッシュ | URLごとに明示した`Cache-Control`、表現別ETag、Last-Modified、条件付きGET/HEAD、Range |
| open/stat回避 | 登録された不変資材を起動時にメモリへ読み込む。元ファイルとgzip版のメモリ量を見積もる |
| ストリーミング | SSEのFlushとWebSocketのHijackをアクセスログwrapper越しに維持。Goに応答bufferingを追加しない |
| アクセスログ | 型付き非同期JSON、応答サイズ・実body読取量・接続再利用・TLS再利用・操作識別値を記録 |

nginxのworker単位の上限とGoのプロセス単位の上限は同じ容量を意味しません。接続・FD・メモリ・CPUと拒否や待ちを実測して調整します。upstreamのidle接続もFDを消費します。転送の再試行は受理済み更新を重複させないよう仕様に基づいて決めます。

## bootstrapで決める内容

1. `LISTEN_PORT`を公開portへ合わせ、必要なら`TLS_CERT_FILE`・`TLS_KEY_FILE`にサーバー上の既存パスを指定する。秘密鍵はGit・生成物へコピーしない。実行ユーザーが証明書と秘密鍵を読み取れる権限を確認する。unitには低いportを非rootで使うための`CAP_NET_BIND_SERVICE`を設定済み。
2. 公開範囲、更新・削除・初期化後の可視性を確認し、不変の公開資材だけを`HTTP_STATIC_ROUTES`へ登録する。認可付きAPIや同一URLで更新されるデータを一律に長期キャッシュしない。未登録URLは業務routerが処理する。
3. たとえば`public/assets/`を不変の`/assets/`へ対応させる場合、次を使う。

```yaml
  HTTP_STATIC_ROUTES: '[{"prefix":"/assets/","directory":"/home/isucon/public/assets","cache_control":"public, max-age=86400"}]'
```

`immutable`や1年の期限は、URLにcontent hash等があり、内容が変わらない場合にだけ指定する。HTMLや同一URLの画像は個別に判断する。prefixは末尾`/`を付ける。ディレクトリ一覧、隠しファイル、symlink、`.gz`への直接アクセスは公開しない。

`task gen`が先に`build-contest-assets`で必要な静的ビルドを行い、`STATIC_SOURCE_DIR`（既定`public/`）から`STATIC_DIR`（既定`generated/public/`）へ生成し、古いgzip版も置き換える。`task deploy`がHTTP設定・元ファイル・gzip版を配布してGoを再起動する。動的アップロードや永続ファイルはこの削除・置換対象から分離する。起動時にgzipを展開して元ファイルとの一致を確認するため、古い圧縮版は起動エラーになる。

## ホストごとの設定

`APP_HOSTS`が配置先、`APP_TRAFFIC_HOSTS`が公開入口です。`HTTP_HOST_CONFIG`のmapでホストごとに`address`、TLSファイル、`max_connections`、`static_routes`を上書きできます。`task gen`が`generated/http/<host>.json`を作り、配布後は`/home/isucon/http.json`を読みます。転送先やデータ分割は業務・通信要件からbootstrapで設計し、最初から多段proxyや3台分散を追加しません。

## 公開入口での確認

初回ベンチ前に、実際の公開URLで通常GET・HEAD、gzip対応/非対応、`gzip;q=0`、Content-Encoding、Vary、Cache-Control、ETagによる304、元ファイルとの内容一致を確認します。TLSでは証明書・HTTP/2、SSEでは接続中のイベント到着を確認します。経路を増やした場合も同じ確認を更新します。

`task test-app`はローカルでTLS/HTTP/2・静的配信・304・Range・SSE flush・計測分離を検証します。`task test-http`は資材再生成時の内容一致と古いgzip版の削除を確認します。`task check-network`は配布したホストごとの待受・計測port、`task check-roles`はnginx等の停止を確認します。これらは競技固有の公開入口の確認や公式ベンチの代替ではありません。

アクセスログは[Goのアクセスログ](../measurement/go-access-log.md)に従い、alp・DuckDB・操作遷移・dashboardまで接続済みです。APIレイテンシだけでなく、キャッシュで不要になった要求や次の操作の開始・完了も比較します。

`task initialize`は配布済みHTTP設定から接続先とHTTP/HTTPSを選び、対象ホスト上から直接呼び出します。ローカル接続の証明書名不一致だけを避けるため、この初期化要求では証明書検証を省略します。公開入口の検証は正規hostnameと証明書で行います。
