# nginx（nginx.conf）

nginx の設定。リポジトリでは `nginx/nginx.conf`、`nginx/conf.d/`、`nginx/sites-enabled/` などを管理し、
`task deploy-nginx` で `NGINX_HOSTS` に配布する。反映前に vhost、TLS、ログ形式、upstream を確認し、
資料の設定を既存ブロックへ統合する。

以下の `events {}` と `http {}` は既存の nginx.conf の各ブロックへ統合する。同じコンテキストを
複数追加しない。

## ファイルディスクリプタと接続数

```nginx
worker_processes auto;
worker_rlimit_nofile 16384;

events {
  worker_connections 2304;
  multi_accept on;
}
```

- `worker_connections` は worker 1 プロセスあたりの同時接続数で、クライアント側だけでなく upstream 側の
  接続も数える。worker 数を掛けた値がそのまま処理可能な HTTP リクエスト数になるわけではない。
- `worker_rlimit_nofile=16384` は worker ごとの FD 上限を `worker_connections=2304` より大きくし、
  upstream 接続、ログ、TLS などにも余裕を設ける。systemd 側の上限がこれより低い場合は、サービス側も設定する。
- `worker_processes auto` の worker 数はホストの CPU 数などから決まる。接続数と FD 上限は各 worker に適用される。

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
配信しない構成では `sendfile` と `tcp_nopush` は静的ファイル配信に効果を持たない。

## クライアント接続の keepalive request 上限

```nginx
http {
  keepalive_requests 1000000;
}
```

`keepalive_requests` は、一つのクライアント keepalive 接続で処理するリクエスト数の上限である。上限に達すると
nginx が接続を閉じるため、クライアントがリクエストを続ける場合は TCP 接続と TLS handshake が再度必要になる。
`1000000` は接続ごとの上限を高くし、上限到達による再接続を減らすための有限値である。

これはクライアントと nginx の間の設定であり、次節の upstream keepalive とは別である。

大きい値は接続単位のメモリーを長く保持する。反映後はレスポンスの正当性に加えて、worker の FD 使用量、
メモリー、接続エラーを確認する。

## nginx と upstream の keepalive

upstream keepalive は、nginx が upstream サーバーへのアイドル接続を再利用する仕組みである。`keepalive` の
`keepalive 60` は同時リクエスト数ではなく、worker ごとに保持するアイドル接続数の上限である。

```nginx
keepalive 60;
```

`keepalive` は既存の `upstream` ブロックへ追加する。その upstream を使う既存の `location` には
次の2行を追加する。

```nginx
proxy_http_version 1.1;
proxy_set_header Connection "";
```

upstream の接続先と `proxy_pass` はアプリケーションの配置・ルーティングに合わせて維持する。

upstream のアプリケーションが keepalive を受け付ける必要がある。Go の `net/http` サーバーは通常これに対応
しているが、アプリケーション側で keepalive を無効化していないこと、read/write timeout で早期切断しないことを
確認する。

`proxy_http_version 1.1` と `proxy_set_header Connection ""` は upstream keepalive のための組み合わせで
あり、片方だけを変更しない。`proxy_read_timeout` は keepalive の設定ではなく応答待ちのタイムアウトなので、
遅いエンドポイントに必要な場合だけ設定する。長くしすぎると、詰まったリクエストが接続や worker を占有する。

`proxy_pass` は `location` の末尾スラッシュの有無で URI の書き換え結果が変わる。既存の API ルーティングを
維持するため、既存の `proxy_pass` とリクエストパスを確認する。

## multi_accept

`multi_accept on` はイベント通知1回で複数の待機接続を受け付ける設定である。バースト時の接続受付を改善する
場合がある一方、worker が受付処理に偏ることもある。設定は上記の `events` ブロックに含める。

## このリポジトリでの注意点と確認

`nginx/conf.d/upstream.conf` は `task gen` の生成物である。`APP_TRAFFIC_HOSTS` を変更する
場合は [`Taskfile.yml`](../../Taskfile.yml) の役割定義を編集し、生成後の upstream を直接編集しない。

`task deploy-nginx` は設定を配布したあと `nginx -t` を実行して reload する。反映後は設定の読み込みとサービスの
状態を確認する。

```shell
sudo nginx -T
sudo systemctl is-active nginx
```

upstream keepalive の実効状態は、nginx と app 側の接続数、`upstream_response_time`、エラー率で確認する。
