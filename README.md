# 最初にやること

## SSHの設定

`~/.ssh/config` のサンプル

```ssh-config
Host isucon-1
  Hostname 13.231.219.177
  IdentityFile ~/.ssh/isucon-practice.pem
  User ubuntu
  RequestTTY yes
  StrictHostKeyChecking no

Host isucon-2
  Hostname 18.179.30.73
  IdentityFile ~/.ssh/isucon-practice.pem
  User ubuntu
  RequestTTY yes
  StrictHostKeyChecking no

Host isucon-3
  Hostname 54.178.138.79
  IdentityFile ~/.ssh/isucon-practice.pem
  User ubuntu
  RequestTTY yes
  StrictHostKeyChecking no
```

sshしつつユーザー切り替え

```shell
ssh isucon-1 "sudo -i -u isucon"
```
## Shell環境を各サーバーにセットアップ

サーバーでShellとNeovim環境をセットアップ。全てのサーバーで実行する

```shell
make setup-shell SETUP_HOST=isucon-1
ssh isucon-1 "sudo -i -u isucon"
sudo passwd isucon
./setup.sh
```

## Goのビルドのセットアップ

Makefileでbuildのターゲットを準備すること

```makefile
build:
	go build -o isuconquest ./... 
```

## Nginx/MySQL/Webappをローカルにコピーして、Git管理にする

Makefileの以下をアプリ名に書換えてから、makeを実行する

```Makefile
APP_NAME:=isuports
```

```shell
make setup-sysctl
make setup-nginx
make setup-mysql
make setup-webapp
```

## Docker剥がし

`etc/systemd/system/$(サービス名) `に以下のように追記する。Dockerで定義されていた環境変数は、Environmentで定義する。

```ini
WorkingDirectory=/home/isucon/webapp/go
ExecStart=/home/isucon/webapp/go/isuports
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

## ログと計測の準備

**これらの設定は最終的に元に戻すこと**

これらの設定を行えば、`make before-bench` をベンチマーク実行前、`make after-bench` をベンチマーク実行後にそれぞれ実行することで、計測結果がそれぞれ以下に格納される。

- alp
  - nginxのアクセスログ。alpで解析する
- slowquery
  - mysqlのslowquery。pt-query-digestなどで解析する
- profile
  - golangのprofile。pdfファイルが出力される

### Nginxでalp用のログ出力

`nginx/nginx.conf` でログ出力を以下のように書き換える

```nginx
http {
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

  access_log /var/log/nginx/access.log json;
```

### MySQLのスロークエリ

`mysql/mysql.conf.d/mysqld.cnf` で以下のように書く。

```ini
slow_query_log		= 1
slow_query_log_file	= /var/log/mysql/mysql-slow.log
long_query_time = 0
```

### Goのprofile

以下のようにProfileの設定をする。

```go
package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"runtime/pprof"
	"syscall"
)

var cpuprofile = flag.String("cpuprofile", "", "write cpu profile to file")

func main() {
	flag.Parse()

	if *cpuprofile != "" {
		f, err := os.Create(*cpuprofile)
		if err != nil {
			log.Fatal(err)
		}

		err = pprof.StartCPUProfile(f)
		if err != nil {
			log.Fatal(err)
		}

		go func() {
			sig := make(chan os.Signal, 1)
			signal.Notify(sig, syscall.SIGTERM, syscall.SIGINT, syscall.SIGQUIT)
			<-sig
			pprof.StopCPUProfile()
			f.Close()
			os.Exit(0)
		}()
	}

	Run()
}
```

systemdでファイル名を指定する

```ini
ExecStart=/home/isucon/webapp/go/isuports -cpuprofile cpu.pprof
```

pdf出力のためgarphvizをinstall

```shell
brew install graphviz
```

また、以下でinteractiveに解析することも出来る

```bash
go tool pprof -source_path $PWD/webapp/go profile/cpu.pprof
```

fgpprofでI/O含めた結果を見る場合

```go
package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"runtime/pprof"
	"syscall"

	"github.com/felixge/fgprof"
)

var cpuprofile = flag.String("cpuprofile", "", "write cpu profile to file")
var fcpuprofile = flag.String("fcpuprofile", "", "write fgprof cpu profile to file")

func main() {
	flag.Parse()

	if *cpuprofile != "" {
		cpu, err := os.Create(*cpuprofile)
		if err != nil {
			log.Fatal(err)
		}

		var fcpu *os.File
		var stop func() error
		if *fcpuprofile != "" {
			fcpu, err = os.Create(*fcpuprofile)
			if err != nil {
				log.Fatal(err)
			}
			stop = fgprof.Start(fcpu, fgprof.FormatPprof)
			if err != nil {
				log.Fatal(err)
			}
		}

		err = pprof.StartCPUProfile(cpu)
		if err != nil {
			log.Fatal(err)
		}

		go func() {
			sig := make(chan os.Signal, 1)
			signal.Notify(sig, syscall.SIGTERM, syscall.SIGINT, syscall.SIGQUIT)
			<-sig

			pprof.StopCPUProfile()
			cpu.Close()

			if stop != nil {
				stop()
				fcpu.Close()
			}

			os.Exit(0)
		}()
	}

	Run()
}
```

# 必要に応じてやること

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

```ini
# bind-address		= 127.0.0.1
```

アプリのsystemdの設定で環境変数を対象のサーバーのIPアドレスに置換える

```ini
Environment=ISUCON_DB_HOST=172.31.32.96
Environment=ISUCON_DB_PORT=3306
Environment=ISUCON_DB_USER=isucon
Environment=ISUCON_DB_PASSWORD=isucon
Environment=ISUCON_DB_NAME=isucon
```

MySQLのisuconユーザーがlocalhost以外からの接続を受け入れているか確認する。%になっていたらOK

```sql
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

なっていなかったら、以下のコマンドでユーザーと権限を追加する

```sql
CREATE USER "isucon"@"%" IDENTIFIED BY "isucon";
GRANT ALL PRIVILEGES ON *.* TO "isucon"@"%";
```

## pgoを有効にする

以下のようにmake targetを書き換える

```makefile
isuports:
	cp cpu.pprof default.pgo || true
	go build -o isuports -pgo=auto ./... 
```

## ER図を出力

サーバーで以下を実行しtblsをインストールし、ドキュメントを出力する。isuportsの部分は対象のDB名なのでアプリの名称によって、適宜変えること。

```shell
brew install k1LoW/tap/tbls
source ~/.zshrc
tbls doc mysql://${USER}:${PASS}@localhost:3306/isuports ./dbdoc
```

その後、ローカルで以下を実行して、ローカルにファイルをコピーして、git管理にする

```shell
mkdir -p dbdoc
rsync -az -e ssh ubuntu@isucon-1:/home/isucon/dbdoc/ dbdoc/ --rsync-path="sudo rsync"
```

## Unix Domain Socket

nginx.confで以下のように設定

```nginx
upstream webapp {
  server unix:/tmp/webapp.sock;
}

server {
  location ~ ^/(api|initialize) {
    proxy_set_header Host $host;
    proxy_read_timeout 600;
    proxy_pass http://webapp;
  }
```

Go言語で以下のように書く

```go
socketFile := "/tmp/webapp.sock"
if _, err := os.Stat(socketFile); err == nil {
    os.Remove(socketFile)
}

listener, err := net.Listen("unix", socketFile)
if err != nil {
    e.Logger.Fatalf("failed to listen: %v", err)
    return
}
err = os.Chmod(socketFile, 0777)
if err != nil {
    e.Logger.Fatalf("failed to chmod: %v", err)
    return
}
e.Listener = listener

server := new(http.Server)
if err := e.StartServer(server); err != nil {
    e.Logger.Fatalf("failed to serve: %v", err)
}
```

# 秘伝のタレ

## nginx.conf

`worker_rlimit_nofile`は`worker_connections`の4倍程度で設定する

```nginx
worker_rlimit_nofile  4096;
events {
  worker_connections 1024;
}
```


基本設定

```nginx
http {
  sendfile    on;
  tcp_nopush  on;
  tcp_nodelay on;
  types_hash_max_size 2048;
  server_tokens    off;
}
```

nginxとupstreamのkeepalive設定。app側も対応必要（Go言語ならデフォルトでOK）

```nginx
upstream app {
  server 192.100.0.1:5000;
  keepalive 60;
}

server {
  location /api/ {
    proxy_set_header Host $host;
    proxy_read_timeout 600;
    proxy_pass http://app;

    # この二つの設定はkeepaliveに必須
    proxy_http_version 1.1;
    proxy_set_header Connection "";
  }
}
```

multi_accept_onにしておく

```nginx
events {
	worker_connections 2304;
	multi_accept on;
}
```

## mysqld.cnf

ディスクイメージをメモリー上にバッファする

```ini
innodb_buffer_pool_size = 1GB
innodb_flush_log_at_trx_commit = 2
innodb_flush_method = O_DIRECT
```

isuconでクラスタ構成を使わない場合disable-log-binを1にする

```ini
disable-log-bin = 1
```

接続数を上げる

```ini
max_connections=10000
```

```go
adminDB.SetMaxOpenConns(50)
```

## ファイルディスクリプタの上限を増やす


MySQLの場合は、`etc/systemd/system/mysql.service.d/limits.conf` で以下のように書いて、`make deploy-mysql`

```ini
[Service]
LimitNOFILE=1006500
```
## カーネルパラメーター

`etc/systemd/sysctl.conf` で以下を書いて、`make deploy-sysctl` を実行で全サーバーに適用

```ini
net.core.somaxconn=65535
net.ipv4.tcp_max_syn_backlog=65535
net.ipv4.ip_local_port_range=10000 60999
net.ipv4.tcp_tw_reuse=1
net.core.rmem_max=16777216
net.core.wmem_max=16777216
```