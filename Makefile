SSH_USER=ubuntu
SSH_HOST=isucon-no-command

setup-files:
	rsync -az -e ssh setup.sh $(SSH_USER)@$(SSH_HOST):/home/isucon/ --rsync-path="sudo rsync"
	rsync -az -e ssh Brewfile $(SSH_USER)@$(SSH_HOST):/home/isucon/ --rsync-path="sudo rsync"
	rsync -az -e ssh Makefile $(SSH_USER)@$(SSH_HOST):/home/isucon/ --rsync-path="sudo rsync"
