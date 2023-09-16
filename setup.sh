#!/bin/bash

echo isucon | sudo passwd isucon
/bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"
eval "$(/home/linuxbrew/.linuxbrew/bin/brew shellenv)"
sudo apt-get install build-essential
brew install gcc
curl -fsSL https://raw.githubusercontent.com/EkeMinusYou/dotfiles/main/install.sh | /bin/bash
fnm use 20
brew bundle --file Brewfile
$(brew --prefix)/opt/fzf/install
sudo chsh -s $(which zsh) isucon
zsh
