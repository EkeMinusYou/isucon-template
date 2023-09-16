USER:=isucon
PASS:=isucon
ISU1:=isucon12-qualify
ISU2:=
ISU3:=

setup:
	echo 'eval "$(/home/linuxbrew/.linuxbrew/bin/brew shellenv)"' >> $(HOME)/.profile
	/home/linuxbrew/.linuxbrew/bin/brew shellenv
	sudo apt-get install build-essential
	brew install gcc
	curl -fsSL https://raw.githubusercontent.com/EkeMinusYou/dotfiles/main/install.sh | /bin/bash
	brew bundle --file Brewfile
	$(brew --prefix)/opt/fzf/install
	sudo chsh -s $(shell which zsh) isucon
	zsh
