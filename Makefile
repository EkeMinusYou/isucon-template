USER:=isucon
PASS:=isucon
ISU1:=isu
ISU2:=
ISU3:=

setup:
	sudo apt-get install build-essential
	brew install gcc
	curl -fsSL https://raw.githubusercontent.com/EkeMinusYou/dotfiles/main/install.sh | /bin/bash
	fnm use 20
	brew bundle --file Brewfile
	$(shell brew --prefix)/opt/fzf/install
	sudo chsh -s $(shell which zsh) isucon
	zsh
