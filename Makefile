SSH_USER:=ubuntu
ISUCON_USER:=isucon
APP_NAME:=isuports

NGINX_HOST:=isucon-1
WEBAPP_HOST:=isucon-2
MYSQL_HOST:=isucon-3

.PHONY: setup setup-nginx setup-mysql setup-webapp deploy-nginx deploy-mysql
setup:
ifndef SETUP_HOST
	@echo "ERROR: SETUP_HOST is not defined\n"
	@exit 1
endif
	rsync -az -e ssh setup.sh $(SSH_USER)@$(SETUP_HOST):/home/$(ISUCON_USER)/ --rsync-path="sudo rsync"
	rsync -az -e ssh Brewfile $(SSH_USER)@$(SETUP_HOST):/home/$(ISUCON_USER)/ --rsync-path="sudo rsync"
	ssh $(SSH_USER)@$(SETUP_HOST) "sudo chmod +x /home/$(ISUCON_USER)/setup.sh"

setup-nginx:
	rsync -az -e ssh $(SSH_USER)@$(NGINX_HOST):/etc/nginx/ nginx/ --rsync-path="sudo rsync"
	git add .
	git commit -m "nginx"

setup-webapp:
	rsync -az -e ssh $(SSH_USER)@$(WEBAPP_HOST):/home/$(ISUCON_USER)/webapp/ webapp --rsync-path="sudo rsync"
	mkdir -p etc/systemd/system
	rsync -az -e ssh $(SSH_USER)@$(WEBAPP_HOST):/etc/systemd/system/$(APP_NAME).service etc/systemd/system/ --rsync-path="sudo rsync"
	git add .
	git commit -m "webapp go"

setup-mysql:
	rsync -az -e ssh $(SSH_USER)@$(MYSQL_HOST):/etc/mysql/ mysql/ --rsync-path="sudo rsync"
	git add .
	git commit -m "mysql"

deploy: deploy-nginx deploy-webapp deploy-mysql

deploy-nginx:
	rsync -az -e ssh nginx/ $(SSH_USER)@$(NGINX_HOST):/etc/nginx/ --rsync-path="sudo rsync"
	ssh $(SSH_USER)@$(NGINX_HOST) "sudo systemctl reload nginx"
	ssh $(SSH_USER)@$(NGINX_HOST) "sudo systemctl restart nginx"
	ssh $(SSH_USER)@$(NGINX_HOST) "sudo mv /var/log/nginx/access.log /var/log/nginx/access.log.`date +%Y%m%d-%H%M%S`"
	ssh $(SSH_USER)@$(NGINX_HOST) "sudo nginx -s reopen"

deploy-webapp:
	rsync -az -e ssh webapp $(SSH_USER)@$(WEBAPP_HOST):/home/$(ISUCON_USER)/ --rsync-path="sudo rsync" --delete
	ssh $(SSH_USER)@$(WEBAPP_HOST) "sudo chown -R $(ISUCON_USER):$(ISUCON_USER) /home/$(ISUCON_USER)/webapp"
	rsync -az -e ssh etc/systemd/system/$(APP_NAME).service $(SSH_USER)@$(WEBAPP_HOST):/etc/systemd/system/ --rsync-path="sudo rsync"
	ssh $(SSH_USER)@$(WEBAPP_HOST) "sudo -i -u $(ISUCON_USER) /home/linuxbrew/.linuxbrew/bin/zsh -c 'source ~/.zshrc && make -C webapp/go $(APP_NAME)'"
	ssh $(SSH_USER)@$(WEBAPP_HOST) "sudo systemctl daemon-reload"
	ssh $(SSH_USER)@$(WEBAPP_HOST) "sudo systemctl restart $(APP_NAME)"

deploy-mysql:
	rsync -az -e ssh mysql/ $(SSH_USER)@$(MYSQL_HOST):/etc/mysql/ --rsync-path="sudo rsync"
	ssh $(SSH_USER)@$(MYSQL_HOST) "sudo systemctl restart mysql"
