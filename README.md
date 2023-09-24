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

サーバーでShellとNeovim環境をセットアップ

```bash
make setup SETUP_HOST=isucon-1
ssh isucon-1 "sudo -i -u isucon && $SHELL"
sudo passwd isucon
./setup.sh
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

etc/systemd/system/$(サービス名) に以下のように追記する

```bash
WorkingDirectory=/home/isucon/webapp/go
ExecStart=/home/isucon/webapp/go/isuports
```

また、環境変数は以下のように定義する

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
