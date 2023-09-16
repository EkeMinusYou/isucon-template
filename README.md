## Setup

ssh & change user

```bash
ssh $(target)
sudo -i -u $(user)
```

install brew

```bash
sudo passwd $(user) 
/bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"
eval "$(/home/linuxbrew/.linuxbrew/bin/brew shellenv)"
```

copy Makefile and Brewfile

```bash
rsync -az -e ssh Makefile ${root}@isucon:/home/isucon/ --rsync-path="sudo rsync"
rsync -az -e ssh Brewfile ${root}@isucon:/home/isucon/ --rsync-path="sudo rsync"
```
