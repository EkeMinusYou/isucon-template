## SSH Sample

```bash
Host isucon-1
  Hostname 13.231.219.177
  IdentityFile ~/.ssh/isucon-practice.pem
  User ubuntu
  RequestTTY yes

Host isucon-2
  Hostname 18.179.30.73
  IdentityFile ~/.ssh/isucon-practice.pem
  User ubuntu
  RequestTTY yes

Host isucon-3
  Hostname 54.178.138.79
  IdentityFile ~/.ssh/isucon-practice.pem
  User ubuntu
  RequestTTY yes
```

## Setup

サーバーでShellとNeovim環境をセットアップ。全てのサーバーで実行する

```bash
make setup SETUP_HOST=isucon-1
ssh isucon-1 "sudo -i -u isucon && $SHELL"
sudo passwd isucon
./setup.sh
```

Nginx/MySQL/Webappをローカルにコピーして、Git管理にする

※ Makefileの以下をアプリ名に書換えてから、makeを実行する

```Makefile
APP_NAME:=isuports
```

```bash
make setup-nginx
make setup-mysql
make setup-webapp
```

## SSH

`~/.ssh/config` のサンプル

```bash
Host isucon-1
  Hostname 13.231.219.177
  IdentityFile ~/.ssh/isucon-practice.pem
  User ubuntu
  RequestTTY yes

Host isucon-2
  Hostname 18.179.30.73
  IdentityFile ~/.ssh/isucon-practice.pem
  User ubuntu
  RequestTTY yes

Host isucon-3
  Hostname 54.178.138.79
  IdentityFile ~/.ssh/isucon-practice.pem
  User ubuntu
  RequestTTY yes
```

sshしつつユーザー切り替え

```bash
ssh isucon-1 "sudo -i -u isucon && $SHELL"
```

## Docker剥がし

`etc/systemd/system/$(サービス名) `に以下のように追記する

```bash
WorkingDirectory=/home/isucon/webapp/go
ExecStart=/home/isucon/webapp/go/isuports
```

また、元々はDockerで定義されていた環境変数は、`etc/sytemd/system/${サービス名}` で以下のように定義する

```bash
Environment=ISUCON_DB_HOST=192.168.0.12
Environment=ISUCON_DB_PORT=3306
Environment=ISUCON_DB_USER=isucon
Environment=ISUCON_DB_PASSWORD=isucon
Environment=ISUCON_DB_NAME=isuports
```

webapp/go配下のビルドするMakefileのサンプル

```Makefile
isuports:
	go build -o isuports ./...
```

## Nginxの向き先を変える

`nginx/sites-available/${サービス名}` の以下を書き変える。proxy_passで `127.0.0.1` となっている箇所を、対象のプライベートIPに変更する。


```conf
  location / {
    try_files $uri /index.html;
  }

  location ~ ^/(api|initialize) {
    proxy_set_header Host $host;
    proxy_read_timeout 600;
    proxy_pass http://127.0.0.1:3000;
  }

  location /auth/ {
    proxy_set_header Host $host;
    proxy_pass http://127.0.0.1:3001;
  }
```

## 別サーバーのDBにアクセスする

対象のサーバーの`mysql/mysql.conf.d/mysqld.confのbind-address`を無効にする

```
# bind-address		= 127.0.0.1
```

アプリのsystemdの設定で環境変数を対象のサーバーのIPアドレスに置換える

```
Environment=ISUCON_DB_HOST=172.31.32.96
Environment=ISUCON_DB_PORT=3306
Environment=ISUCON_DB_USER=isucon
Environment=ISUCON_DB_PASSWORD=isucon
Environment=ISUCON_DB_NAME=isucon
```

MySQLのisuconユーザーがlocalhost以外からの接続を受け入れているか確認する。%になっていたらOK

```
mysql -u isucon -p
mysql> SELECT user, host FROM mysql.user;
+------------------+-----------+
| user             | host      |
+------------------+-----------+
| isucon           | %         |
| debian-sys-maint | localhost |
| mysql.infoschema | localhost |
| mysql.session    | localhost |
| mysql.sys        | localhost |
| root             | localhost |
+------------------+-----------+
6 rows in set (0.00 sec)
```

## ログと計測の準備

### Nginx

`nginx/nginx.conf` でログ出力を以下のように書き換える

```
  log_format json escape=json '{"time":"$time_local",'
                              '"host":"$remote_addr",'
                              '"forwardedfor":"$http_x_forwarded_for",'
                              '"req":"$request",'
                              '"status":"$status",'
                              '"method":"$request_method",'
                              '"uri":"$request_uri",'
                              '"body_bytes":$body_bytes_sent,'
                              '"referer":"$http_referer",'
                              '"ua":"$http_user_agent",'
                              '"request_time":$request_time,'
                              '"cache":"$upstream_http_x_cache",'
                              '"runtime":"$upstream_http_x_runtime",'
                              '"response_time":"$upstream_response_time",'
                              '"vhost":"$host"}';
```

```
access_log /var/log/nginx/access.log json;
```

### MySQL

`mysql/mysql.conf.d/mysqld.cnf` の以下の部分を有効にする

```
slow_query_log		= 1
slow_query_log_file	= /var/log/mysql/mysql-slow.log
long_query_time = 0
```
