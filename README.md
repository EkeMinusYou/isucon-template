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
make setup-files
```

ssh

```bash
ssh isucon
```
