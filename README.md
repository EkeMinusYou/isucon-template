## Required

ssh sample

```bash
Host isucon-1
  Hostname 172.31.41.143
  IdentityFile ~/.ssh/isucon-practice.pem
  User ubuntu
  RequestTTY yes

Host isucon-2
  Hostname 172.31.44.122
  IdentityFile ~/.ssh/isucon-practice.pem
  User ubuntu
  RequestTTY yes

Host isucon-3
  Hostname 172.31.32.96
  IdentityFile ~/.ssh/isucon-practice.pem
  User ubuntu
  RequestTTY yes
```

## Setup

copy files


```bash
make setup
ssh isucon
sudo passwd isucon
./setup.sh
```

ssh and login

```bash
ssh isucon-1 "sudo -i -u isucon && $SHELL"
```

add systemctl webapp sample

```bash
WorkingDirectory=/home/isucon/webapp/go
ExecStart=/home/isucon/webapp/go/isuports
```
