# nginx（nginx.conf）

nginx の設定例。リポジトリでは `nginx/nginx.conf`、`nginx/conf.d/`、`nginx/sites-enabled/` などを管理し、
`task deploy-nginx` で `NGINX_HOSTS` に配布する。反映前に必ず現在の vhost、TLS、ログ形式、upstream を確認し、
必要な設定だけを既存ブロックへ統合する。

以下の `events {}` と `http {}` は例として一つずつ示している。既存の nginx.conf に同じコンテキストを
二つ追加するのではなく、既存ブロックの該当ディレクティブを編集する。

## ファイルディスクリプタと接続数

```nginx
worker_processes auto;
worker_rlimit_nofile 4096;

events {
  worker_connections 1024;
}
```

- `worker_connections` は worker 1 プロセスあたりの同時接続数で、クライアント側だけでなく upstream 側の
  接続も数える。worker 数を掛けた値がそのまま処理可能な HTTP リクエスト数になるわけではない。
- `worker_rlimit_nofile` は worker が開けるファイルディスクリプタ上限である。`worker_connections` の約4倍は
  upstream 接続、ログ、TLS などの余裕を持たせる目安であり、固定の必須比率ではない。
- `worker_processes auto` の worker 数はホストの CPU 数などから決まるため、接続数と FD 使用量を実測して値を
  決める。

## 基本設定

```nginx
http {
  sendfile on;
  tcp_nopush on;
  tcp_nodelay on;
  types_hash_max_size 2048;
  server_tokens off;
}
```

`sendfile` は静的ファイル配信、`tcp_nopush` は主に `sendfile` と組み合わせた送信、`tcp_nodelay` は
keepalive 接続での小さい応答の待ちを減らすための設定である。`types_hash_max_size` は MIME type の検索表の
サイズ、`server_tokens off` はエラーページなどに nginx のバージョンを表示しない設定である。静的ファイルを
配信しない構成や、別の TLS・レスポンス設定がある場合は、変更前後の通信量とレイテンシーを比較する。

## クライアント接続の keepalive request 上限

```nginx
http {
  keepalive_requests 1000000;
}
```

`keepalive_requests` は、一つのクライアント keepalive 接続で処理するリクエスト数の上限である。上限に達すると
nginx が接続を閉じるため、クライアントがリクエストを続ける場合は TCP 接続と TLS handshake が再度必要になる。
短時間のベンチマークで同じ接続が大量のリクエストを送る構成では、十分に大きい有限値を設定して
server-forced reconnect を走行中に発生させない。

nginx のバージョンによって既定値が異なるため、値を追加する前に `nginx -T` と access log の
`$connection`・`$connection_requests` を使い、同じリクエスト数で接続が繰り返し終わっていないか確認する。
これはクライアントと nginx の間の設定であり、次節の upstream keepalive とは別である。

大きい値は接続単位のメモリーを長く保持するため、無期限を意図した値として一般サービスへそのまま適用しない。
反映後は HTTPS、TLS、レスポンスの correctness に加えて、worker の FD 使用量、メモリー、接続エラー、接続の偏りを
確認する。`keepalive_timeout`、TLS session cache、upstream keepalive は別の変更として扱い、同時に変更しない。

## nginx と upstream の keepalive

upstream keepalive は、nginx が upstream サーバーへのアイドル接続を再利用する仕組みである。`keepalive` の
値は同時リクエスト数ではなく、worker ごとに保持するアイドル接続数なので、worker 数と upstream サーバー数に
応じて設定する。

```nginx
upstream app {
  server 192.100.0.1:5000;
  keepalive 60;
}

server {
  location /api/ {
    proxy_set_header Host $host;
    proxy_pass http://app;

    # Both settings are required for upstream keepalive.
    proxy_http_version 1.1;
    proxy_set_header Connection "";
  }
}
```

upstream のアプリケーションが keepalive を受け付ける必要がある。Go の `net/http` サーバーは通常これに対応
しているが、アプリケーション側で keepalive を無効化していないこと、read/write timeout で早期切断しないことを
確認する。

`proxy_http_version 1.1` と `proxy_set_header Connection ""` は upstream keepalive のための組み合わせで
あり、片方だけを変更しない。`proxy_read_timeout` は keepalive の設定ではなく応答待ちのタイムアウトなので、
遅いエンドポイントに必要な場合だけ設定する。長くしすぎると、詰まったリクエストが接続や worker を占有する。

`proxy_pass` は `location` の末尾スラッシュの有無で URI の書き換え結果が変わる。既存の API ルーティングを
壊さないよう、設定例の location をそのまま貼らず、現行の `nginx/sites-enabled/` とリクエストパスを確認する。

## multi_accept

```nginx
events {
  worker_connections 2304;
  multi_accept on;
}
```

`multi_accept on` はイベント通知1回で複数の待機接続を受け付ける設定である。バースト時の接続受付を改善する
場合がある一方、worker が受付処理に偏ることもあるため、CPU 使用率、接続エラー、リクエストレイテンシーを
比較して採否を判断する。

## このリポジトリでの注意点と確認

`nginx/conf.d/upstream.conf` は `task gen` の生成物である。`APP_TRAFFIC_HOSTS` を変更する
場合は [`Taskfile.yml`](../../Taskfile.yml) の役割定義を編集し、生成後の upstream を直接編集しない。

`task deploy-nginx` は設定を配布したあと `nginx -t` を実行して reload する。反映後は設定の読み込みとサービスの
状態を確認する。

```shell
sudo nginx -T
sudo systemctl is-active nginx
```

upstream keepalive の効果は、nginx の接続数だけでなく app 側の接続数、`upstream_response_time`、エラー率、
CPU 使用率を同じ条件で比較して判断する。
