## Required

ssh sample

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

copy files


```bash
make setup SETUP_HOST=isucon-1
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
