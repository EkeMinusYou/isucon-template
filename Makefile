SSH_USER:=ubuntu
ISUCON_USER:=isucon
SSH_HOST:=isucon-no-command
APP_NAME:=isuports

NGINX_HOST:=isucon-no-command
MYSQL_HOST:=isucon-no-command
WEBAPP_HOST:=isucon-no-command

.PHONY: setup setup-nginx setup-mysql setup-webapp deploy-nginx deploy-mysql
setup:
	rsync -az -e ssh setup.sh $(SSH_USER)@$(SSH_HOST):/home/$(ISUCON_USER)/ --rsync-path="sudo rsync"
	rsync -az -e ssh Brewfile $(SSH_USER)@$(SSH_HOST):/home/$(ISUCON_USER)/ --rsync-path="sudo rsync"
	rsync -az -e ssh Makefile $(SSH_USER)@$(SSH_HOST):/home/$(ISUCON_USER)/ --rsync-path="sudo rsync"
	ssh $(SSH_USER)@$(SSH_HOST) "sudo chmod +x /home/$(ISUCON_USER)/setup.sh"

setup-nginx:
	rsync -az -e ssh $(SSH_USER)@$(SSH_HOST):/etc/nginx/sites-available/$(APP_NAME).conf nginx/sites-available/$(APP_NAME).conf
	rsync -az -e ssh $(SSH_USER)@$(SSH_HOST):/etc/nginx/nginx.conf nginx/nginx.conf
	git add .
	git commit -m "nginx.conf"

setup-mysql:
	rsync -az -e ssh $(SSH_USER)@$(SSH_HOST):/etc/mysql/mysql.conf.d/mysqld.cnf mysql/mysqld.cnf
	git add .
	git commit -m "mysqld.cnf"

setup-webapp:
	rsync -az -e ssh $(SSH_USER)@$(SSH_HOST):/home/$(ISUCON_USER)/webapp/go webapp --rsync-path="sudo rsync"
	git add .
	git commit -m "webapp go"

deploy: deploy-nginx deploy-mysql deploy-webapp

deploy-nginx:
	rsync -az -e ssh nginx/sites-available/$(APP_NAME).conf $(SSH_USER)@$(NGINX_HOST):/etc/nginx/sites-available/$(APP_NAME).conf --rsync-path="sudo rsync"
	rsync -az -e ssh nginx/nginx.conf $(SSH_USER)@$(NGINX_HOST):/etc/nginx/nginx.conf --rsync-path="sudo rsync"
	ssh $(SSH_USER)@$(NGINX_HOST) "sudo systemctl reload nginx"
	ssh $(SSH_USER)@$(NGINX_HOST) "sudo systemctl restart nginx"

deploy-mysql:
	rsync -az -e ssh mysql/mysqld.cnf $(SSH_USER)@$(MYSQL_HOST):/etc/mysql/mysql.conf.d/mysqld.cnf --rsync-path="sudo rsync"
	ssh $(SSH_USER)@$(MYSQL_HOST) "sudo systemctl restart mysql"

deploy-webapp:
	go build -buildsvc=false -o webapp/go/$(APP_NAME) ./webapp/go/...
	rsync -az -e ssh webapp/go/$(APP_NAME) $(SSH_USER)@$(WEBAPP_HOST):/home/$(ISUCON_USER)/webapp/go/$(APP_NAME) --rsync-path="sudo rsync"
	ssh $(SSH_USER)@$(WEBAPP_HOST) "sudo chmod +x /home/$(ISUCON_USER)/webapp/go/$(APP_NAME)"
	ssh $(SSH_USER)@$(WEBAPP_HOST) "sudo systemctl restart $(APP_NAME)"
