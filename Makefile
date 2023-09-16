SSH_USER:=ubuntu
ISUCON_USER:=isucon
SSH_HOST:=isucon-no-command

setup-files:
	rsync -az -e ssh setup.sh $(SSH_USER)@$(SSH_HOST):/home/$(ISUCON_USER)/ --rsync-path="sudo rsync"
	rsync -az -e ssh Brewfile $(SSH_USER)@$(SSH_HOST):/home/$(ISUCON_USER)/ --rsync-path="sudo rsync"
	rsync -az -e ssh Makefile $(SSH_USER)@$(SSH_HOST):/home/$(ISUCON_USER)/ --rsync-path="sudo rsync"
