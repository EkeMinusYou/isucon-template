# nginxの秘伝のタレをGoへ適用する

このテンプレートはGoが公開HTTP/TLSと静的配信を直接処理する。nginx設定の狙いを次のように初期実装へ移している。競技サーバーの元のnginx設定は外部仕様の調査用に`REFERENCE_DIR`へ取得し、稼働構成へ持ち込まない。

| 元の設定・狙い | Goでの初期状態 |
| --- | --- |
| `worker_processes auto` | Go runtimeの標準CPU並列度を使用。固定のGOMAXPROCSを追加しない |
| `worker_rlimit_nofile 65536` | 生成systemd unitの`LimitNOFILE=65536` |
| `worker_connections 32768`, `multi_accept on` | nginx workerの設定は対象外。Goのaccept loop・netpollを使用し、接続数・FD・CPUを実測する |
| `tcp_nodelay on` | GoのTCP接続の標準動作を利用 |
| `sendfile on` | 大きい非圧縮ファイルは`http.ServeContent`。ログwrapperも`io.ReaderFrom`を維持する。TLS・OS等でゼロコピー可否は異なる |
| `tcp_nopush`, `types_hash_max_size`, `server_tokens off` | nginx固有の設定は対象外。MIMEはGoで設定し、nginxのバージョンヘッダーは存在しない |
| client `keepalive_requests 1000000` | HTTP/1で100万要求を上限とし正常応答後にclose。idle timeoutは120秒。HTTP/2の接続管理はnet/httpが担当 |
| upstream `keepalive 1024` | `measurement.NewUpstreamTransport()`は全体・接続先ごとのidle pool上限1024、idle timeout 120秒 |
| upstream `keepalive_requests 100000` | Go標準Transportに同一の設定はない。再利用を有効にし、要求回数だけを理由に1000回で切断しない。必要なら競技側で有限上限を設計する |
| `expires 1d` | 宣言した不変の公開静的prefixだけに`Cache-Control: public, max-age=86400`と`Expires` |
| `open_file_cache max=1024 inactive=60s` | 内容・validator・圧縮済み表現を最大1024件・64MiBまで保持。最も古い利用項目から追い出し、60秒で再検証。8MiB超はファイルから配信 |
| 静的gzip・`gzip_min_length 1024` | 1KiB以上のHTML/CSS/JavaScript/SVGだけを事前圧縮して再利用。JSON APIへ一律適用しない |
| `gzip_vary on` | `Vary: Accept-Encoding`。q値でgzip拒否を尊重し、gzipとidentityのETagを分ける |
| HTTP/2 | TLSのALPNで既定有効。`HTTP2_ENABLED=false`でHTTP/1.1のみへ切替。平文HTTPはHTTP/1。TLS1.2以上 |
| buffered JSON access log | 共通の非同期ロガーで5ms batch・100ms flush、最大4096件queue、欠損・出力失敗を計測 |

`STATIC_URL_PREFIX`は外部仕様で確認した静的URLだけに設定する。既定の`/static/`は開始用の値であり、競技のURLを推測する根拠にはしない。同一URLで更新する画像、HTML、認証付き資材、初期化で置き換わるファイルへ1日の期限を一律適用しない。静的cacheはプロセス内にあるため、`task deploy-static`後の通常restartで更新できる。即時更新や厳密な初期化が必要なものは専用のhandlerとcache無効化を設計する。

HTTP/1 keepalive、FD上限、静的cacheの容量と期限、gzip対象、TCPサンプル間隔は初期値であり最適値ではない。スコア効果の事前実測は導入条件にしないが、仕様上の矛盾と資源上限は確認する。

## 反映と確認

コードは`webapp/go/`と共通の`tools/measurement/app/`、配布値は`Taskfile.yml`を編集し、`task deploy-all`で反映する。systemdのFD上限・bind capabilityは再起動後に実プロセスでも確認する。静的配信は200/304、HEAD・Range、ETag、Cache-Control・Expires、gzipと非圧縮の両表現を検証する。TLSは公式の証明書・信頼点でホスト名と有効期間を検証し、設定したHTTPバージョンを公開応答とアクセスログの`protocol`で確認する。

`task test-app`はローカルHTTP/TLS、HTTP/2の有効・無効、静的配信を検証する。競技固有の公開URL・更新・認可・初期化条件は`tools/contest/validation.json`と競技側テストへ宣言する。アプリ・計測の起動成功だけで競技の正当性まで確認済みとしない。

## 元の設定のリファレンス

- [nginx upstream keepalive](https://nginx.org/en/docs/http/ngx_http_upstream_module.html#keepalive)
- [nginx expires](https://nginx.org/en/docs/http/ngx_http_headers_module.html#expires)
- [Go HTTP server / Transport](https://pkg.go.dev/net/http)
