# nginxのアクセスログ

標準集計の入口は`NGINX_HOSTS`（measurectlの`ingress` role）です。`nginx/common/nginx.conf`のJSONログを`ACCESS_LOG_PATH`へ出し、alp・操作遷移・負荷期間の推定・DuckDBで同じファイルを読みます。Goのpprof・runtimeは`APP_HOSTS`から引き続き収集します。

汎用ベースのnginx設定では応答終了時刻`msec`（Unix秒）、`method`・`uri`・`status`、全体の秒数`response_time`、転送bodyの`body_bytes`、接続ID・接続内要求数・TLS再利用、仮想ホスト`vhost`、転送先・upstream応答時間を記録します。`buffer=32k flush=1s`で書き出し、回収前に2秒待ちます。ベンチ前にはsudoでtruncate・reopenします。ログの親ディレクトリにSSHユーザーの通過権限、ファイルに読み取り権限を与え、`task setup-postflight`と実RUNで回収を確認してください。認証CookieやAuthorizationそのものをログに追加しません。

## Goから補う測定値

Goは応答ヘッダーの確定前に次を設定します。汎用土台の`webapp/go/measurement.go`に`setMeasurementHeaders`を用意しています。bootstrapで認証とbody読み取りの実装箇所へ接続し、未取得の値は渡しません。

- `X-Measurement-Session-ID`: 認証セッションから作る安定した非可逆の識別値。全Goホストで同じセッションが同じ値になるようにする。未認証で識別できない要求では省略する。ログイン成功応答では発行したセッション、ログアウトでは終了したセッションを使う。値に生のtokenを含めない。
- `X-Measurement-Request-Body-Bytes`: 必要な場合だけ、アプリが実際に読み取った非負整数のbyte数。共通moduleの`app.ObserveRequestBody`を利用できる。ストリーミングでヘッダー確定後まで読み取る場合等、最終値を送れないときは省略し、0にしない。

nginxはクライアントから同名ヘッダーを転送せず、Goの応答からだけログへ取り込み、外部への応答では隠します。`request_content_length`は要求のContent-Lengthで、実読取量とは別です。存在しない数値はJSONの`null`にします。静的な直接配信にはGoの測定値はありません。

API分類の正本は`tools/contest/user-transition-routes.json`だけです。`task check-api-routes`で宣言を検査し、alp・操作遷移・DuckDBは同じ分類を使います。RUN開始時の定義を保存し、`task q`の`route_coverage`とRUN比較で未分類・定義欠損・集計件数を確認します。`alp.yml`とSQLへ分類を重複記述しません。長時間応答は終了時に初めて記録されるため、進行中SSEの件数・待ち時間は必要に応じてGoの内部イベントで補います。HTTP/2の接続内並行要求を逐次HTTP/1.1の待ち時間として解釈しません。

Goログを追加する場合は[Goのアクセスログ](go-access-log.md)を参照し、別ファイルへ出します。nginxとGoの同一要求を二重集計せず、nginxログにGo loggerの排出counterを対応させません。追加した`upstream_*`列は生ログに残りますが、標準DuckDBビューはまだ集計しません。
