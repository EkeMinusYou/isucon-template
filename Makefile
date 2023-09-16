USER:=isucon
PASS:=isucon

setup:
	sudo -i -u $(USER)
	echo "${PASS}\n${PASS}\n" | sudo passwd $(USER)
	echo $(PASS) | /bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"
	(echo; echo 'eval "$(/home/linuxbrew/.linuxbrew/bin/brew shellenv)"') >> $HOME/.profile
	eval "$(/home/linuxbrew/.linuxbrew/bin/brew shellenv)"
	sudo apt-get install build-essential
	brew install gcc
	bash -c "$(curl -fsSL https://raw.githubusercontent.com/EkeMinusYou/dotfiles/main/install.sh)"
	brew bundle --file Brewfile
	$(brew --prefix)/opt/fzf/install
