## Required

ssh config

```bash
Host isucon
  Hostname ${host}
  IdentityFile ~/.ssh/isucon.pem
  User ubuntu
  RequestTTY yes
  RemoteCommand sudo -i -u isucon && $SHELL

Host isucon-no-command
  Hostname ${host}
  IdentityFile ~/.ssh/isucon.pem
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

ssh

```bash
ssh isucon
```

add systemctl webapp sample

```bash
WorkingDirectory=/home/isucon/webapp/go
ExecStart=/home/isucon/webapp/go/isuports
```
