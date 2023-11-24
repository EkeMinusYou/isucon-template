SSH_USER:=ubuntu
ISUCON_USER:=isucon
APP_NAME:=isuports

NGINX_HOST:=isucon-1
WEBAPP_HOST:=isucon-1
MYSQL_HOST:=isucon-1

.PHONY: setup-shell
setup-shell:
ifndef SETUP_HOST
	@echo "ERROR: SETUP_HOST is not defined\n"
	@exit 1
endif
	rsync -az -e ssh setup.sh $(SSH_USER)@$(SETUP_HOST):/home/$(ISUCON_USER)/ --rsync-path="sudo rsync"
	rsync -az -e ssh Brewfile $(SSH_USER)@$(SETUP_HOST):/home/$(ISUCON_USER)/ --rsync-path="sudo rsync"
	ssh $(SSH_USER)@$(SETUP_HOST) "sudo chmod +x /home/$(ISUCON_USER)/setup.sh"

.PHONY: setup
setup: setup-sysctl setup-nginx setup-webapp setup-mysql

.PHONY: setup-sysctl
setup-sysctl:
	mkdir -p etc
	rsync -az -e ssh $(SSH_USER)@$(NGINX_HOST):/etc/sysctl.conf etc/ --rsync-path="sudo rsync"
	git add .
	git commit -m "sysctl"

.PHONY: setup-nginx
setup-nginx:
	rsync -az -e ssh $(SSH_USER)@$(NGINX_HOST):/etc/nginx/ nginx/ --rsync-path="sudo rsync"
	git add .
	git commit -m "nginx"

.PHONY: setup-webapp
setup-webapp:
	rsync -v --progress -az -e ssh $(SSH_USER)@$(WEBAPP_HOST):/home/$(ISUCON_USER)/webapp/ webapp --rsync-path="sudo rsync"
	mkdir -p etc/systemd/system
	rsync -az -e ssh $(SSH_USER)@$(WEBAPP_HOST):/etc/systemd/system/$(APP_NAME).service etc/systemd/system/ --rsync-path="sudo rsync"
	git add .
	git commit -m "webapp go"

.PHONY: setup-mysql
setup-mysql:
	rsync -az -e ssh $(SSH_USER)@$(MYSQL_HOST):/etc/mysql/ mysql/ --rsync-path="sudo rsync"
	mkdir -p etc/systemd/system/mysql.service.d
	ssh $(SSH_USER)@$(MYSQL_HOST) "sudo mkdir -p /etc/systemd/system/mysql.service.d"
	ssh $(SSH_USER)@$(MYSQL_HOST) "sudo touch /etc/systemd/system/mysql.service.d/limits.conf"
	rsync -az -e ssh $(SSH_USER)@$(MYSQL_HOST):/etc/systemd/system/mysql.service.d/limits.conf etc/systemd/system/mysql.service.d/ --rsync-path="sudo rsync"
	git add .
	git commit -m "mysql"

.PHONY: setup-google-service-account
setup-google-service-account:
	ssh $(SSH_USER)@$(WEBAPP_HOST) "sudo mkdir -p /home/$(ISUCON_USER)/.config/gcloud"
	rsync -az -e ssh application_default_credentials.json $(SSH_USER)@$(WEBAPP_HOST):/home/$(ISUCON_USER)/.config/gcloud/application_default_credentials.json --rsync-path="sudo rsync"
	ssh $(SSH_USER)@$(MYSQL_HOST) "sudo mkdir -p /home/$(ISUCON_USER)/.config/gcloud"
	rsync -az -e ssh application_default_credentials.json $(SSH_USER)@$(MYSQL_HOST):/home/$(ISUCON_USER)/.config/gcloud/application_default_credentials.json --rsync-path="sudo rsync"
	ssh $(SSH_USER)@$(NGINX_HOST) "sudo mkdir -p /home/$(ISUCON_USER)/.config/gcloud"
	rsync -az -e ssh application_default_credentials.json $(SSH_USER)@$(NGINX_HOST):/home/$(ISUCON_USER)/.config/gcloud/application_default_credentials.json --rsync-path="sudo rsync"

.PHONY: deploy
deploy: deploy-sysctl deploy-nginx deploy-webapp deploy-mysql

.PHONY: deploy-sysctl
deploy-sysctl:
	rsync -az -e ssh etc/sysctl.conf $(SSH_USER)@$(NGINX_HOST):/etc/ --rsync-path="sudo rsync"
	ssh $(SSH_USER)@$(NGINX_HOST) "sudo sysctl -p"
	rsync -az -e ssh etc/sysctl.conf $(SSH_USER)@$(WEBAPP_HOST):/etc/ --rsync-path="sudo rsync"
	ssh $(SSH_USER)@$(WEBAPP_HOST) "sudo sysctl -p"
	rsync -az -e ssh etc/sysctl.conf $(SSH_USER)@$(MYSQL_HOST):/etc/ --rsync-path="sudo rsync"
	ssh $(SSH_USER)@$(MYSQL_HOST) "sudo sysctl -p"

.PHONY: deploy-nginx
deploy-nginx:
	rsync -az -e ssh nginx/ $(SSH_USER)@$(NGINX_HOST):/etc/nginx/ --rsync-path="sudo rsync"
	ssh $(SSH_USER)@$(NGINX_HOST) "sudo systemctl reload nginx"
	ssh $(SSH_USER)@$(NGINX_HOST) "sudo systemctl restart nginx"

.PHONY: deploy-webapp
deploy-webapp:
	rsync -az -e ssh webapp $(SSH_USER)@$(WEBAPP_HOST):/home/$(ISUCON_USER)/ --rsync-path="sudo rsync" --delete
	ssh $(SSH_USER)@$(WEBAPP_HOST) "sudo chown -R $(ISUCON_USER):$(ISUCON_USER) /home/$(ISUCON_USER)/webapp"
	rsync -az -e ssh etc/systemd/system/$(APP_NAME).service $(SSH_USER)@$(WEBAPP_HOST):/etc/systemd/system/ --rsync-path="sudo rsync"
	ssh $(SSH_USER)@$(WEBAPP_HOST) "sudo -i -u $(ISUCON_USER) /home/linuxbrew/.linuxbrew/bin/zsh -c 'source ~/.zshrc && make -C webapp/go build'"
	ssh $(SSH_USER)@$(WEBAPP_HOST) "sudo systemctl daemon-reload"
	ssh $(SSH_USER)@$(WEBAPP_HOST) "sudo systemctl restart $(APP_NAME)"

.PHONY: deploy-mysql
deploy-mysql:
	rsync -az -e ssh mysql/ $(SSH_USER)@$(MYSQL_HOST):/etc/mysql/ --rsync-path="sudo rsync"
	rsync -az -e ssh etc/systemd/system/mysql.service.d/limits.conf $(SSH_USER)@$(MYSQL_HOST):/etc/systemd/system/mysql.service.d/ --rsync-path="sudo rsync"
	ssh $(SSH_USER)@$(MYSQL_HOST) "sudo systemctl daemon-reload"
	ssh $(SSH_USER)@$(MYSQL_HOST) "sudo systemctl restart mysql"

.PHONY: before-bench
before-bench:
	ssh $(SSH_USER)@$(NGINX_HOST) "sudo mv /var/log/nginx/access.log /var/log/nginx/access.log.`date +%Y%m%d-%H%M%S`"
	ssh $(SSH_USER)@$(NGINX_HOST) "sudo nginx -s reopen"
	ssh $(SSH_USER)@$(MYSQL_HOST) "sudo mv /var/log/mysql/mysql-slow.log /var/log/mysql/mysql-slow.log.`date +%Y%m%d-%H%M%S`"
	ssh $(SSH_USER)@$(MYSQL_HOST) "sudo systemctl restart mysql"

.PHONY: after-bench
after-bench:
	mkdir -p alp
	rsync -az -e ssh $(SSH_USER)@$(NGINX_HOST):/var/log/nginx/ alp/ --rsync-path="sudo rsync"
	mkdir -p slowquery
	[ -e "slowquery/pt-query-digest.log" ] && mv slowquery/pt-query-digest.log slowquery/pt-query-digest_`date +%Y%m%d-%H%M%S`.log || true
	rsync -az -e ssh $(SSH_USER)@$(MYSQL_HOST):/var/log/mysql/ slowquery/log/ --rsync-path="sudo rsync"
	pt-query-digest slowquery/log/mysql-slow.log > slowquery/pt-query-digest.log
	mkdir -p profile
	[ -e "profile/cpu.pprof" ] && mv profile/cpu.pprof profile/cpu_`date +%Y%m%d-%H%M%S`.pprof || true
	ssh $(SSH_USER)@$(WEBAPP_HOST) "sudo systemctl stop $(APP_NAME)"
	rsync -az -e ssh $(SSH_USER)@$(WEBAPP_HOST):/home/$(ISUCON_USER)/webapp/go/cpu.pprof profile/ --rsync-path="sudo rsync"  || true
	ssh $(SSH_USER)@$(WEBAPP_HOST) "sudo systemctl start $(APP_NAME)"

.PHONY: clear-cache
clear-cache:
	ssh $(SSH_USER)@$(MYSQL_HOST) "sudo systemctl stop mysql"
	ssh $(SSH_USER)@$(MYSQL_HOST) "sudo rm -f /var/log/mysql/*"
	ssh $(SSH_USER)@$(MYSQL_HOST) "sudo systemctl start mysql"
	ssh $(SSH_USER)@$(MYSQL_HOST) "sudo journalctl --vacuum-size=10M"
	ssh $(SSH_USER)@$(NGINX_HOST) "sudo systemctl stop nginx"
	ssh $(SSH_USER)@$(NGINX_HOST) "sudo rm -f /var/log/nginx/*"
	ssh $(SSH_USER)@$(NGINX_HOST) "sudo systemctl start nginx"
	ssh $(SSH_USER)@$(NGINX_HOST) "sudo journalctl --vacuum-size=10M"
	ssh $(SSH_USER)@$(WEBAPP_HOST) "sudo -i -u $(ISUCON_USER) /home/linuxbrew/.linuxbrew/bin/zsh -c 'source ~/.zshrc && go clean -cache -testcache'"
	ssh $(SSH_USER)@$(WEBAPP_HOST) "sudo journalctl --vacuum-size=10M"
