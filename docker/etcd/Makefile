.PHONY: up net add check help leader remove get put stop-all

# 可修改的默认值
COMPOSE_FILE ?= docker-compose.yml
EP ?= http://etcd1:12379    # 用来运行 etcdctl 的已有节点 client URL
NEW_PORT ?= 42379
ETCD_IMAGE ?= bitnami/etcd:3.5
NET_NAME ?= etcd_default
TOKEN ?= MyEtcd

help:
	@echo "用法示例："
	@echo "  make up                 # 启动 docker-compose 中定义的已有集群"
	@echo "  make add NEW_NAME=etcd4 [NEW_CLIENT_PORT=42379]  # 向集群添加新节点"
	@echo "  make check              # 打印集群成员列表"
	@echo "  make leader             # 查询当前 leader 节点"
	@echo "  make remove NAME=etcd4  # 移除指定节点"
	@echo "  make get KEY=/my/key    # 查询指定键值,如果不填KEY就是获取全部键值"
	@echo "  make stop-all #删除所有以etcd为名称开头的容器"

up:
	@docker-compose -f $(COMPOSE_FILE) up -d

net:
	@echo "创建网络 $(NET_NAME)（如果已存在则忽略）"
	-@docker network create $(NET_NAME) >/dev/null 2>&1 || true
	@echo "把已有容器连接到网络（失败则忽略）"
	-@docker network connect $(NET_NAME) etcd1 >/dev/null 2>&1 || true
	-@docker network connect $(NET_NAME) etcd2 >/dev/null 2>&1 || true
	-@docker network connect $(NET_NAME) etcd3 >/dev/null 2>&1 || true
	@echo "网络准备完毕"

add:
	@if [ -z "$(NEW_NAME)" ]; then \
		echo "错误：请指定 NEW_NAME，例如： make add NEW_NAME=etcd4"; exit 1; \
	fi; \
	NEW_CLIENT_PORT=$${NEW_PORT:-$(NEW_PORT)}; \
	echo "1) 确保已有集群已启动且健康，使用 EXISTING_ENDPOINT=$(EP)"; \
	echo "2) 准备网络并把已有容器连入 $(NET_NAME)"; \
	docker network create $(NET_NAME) >/dev/null 2>&1 || true; \
	for c in etcd1 etcd2 etcd3; do docker network connect $(NET_NAME) $$c >/dev/null 2>&1 || true; done; \
	echo "3) 调用 etcdctl member add 来注册新成员 '$(NEW_NAME)' ..."; \
	docker run --rm --network $(NET_NAME) --env ETCDCTL_API=3 $(ETCD_IMAGE) \
		etcdctl --endpoints=$(EP) member add $(NEW_NAME) --peer-urls=http://$(NEW_NAME):2380 > /tmp/etcd_member_add.$(NEW_NAME) || (echo "etcdctl member add 失败，请检查 EXISTING_ENDPOINT 是否可达"; exit 1); \
	INITIAL_CLUSTER=$$(grep -m1 ETCD_INITIAL_CLUSTER /tmp/etcd_member_add.$(NEW_NAME) | sed -E 's/.*="(.*)".*/\1/'); \
	if [ -z "$$INITIAL_CLUSTER" ]; then echo "未能从 member add 输出获取 ETCD_INITIAL_CLUSTER，请检查 /tmp/etcd_member_add.$(NEW_NAME)"; echo "输出内容如下:"; cat /tmp/etcd_member_add.$(NEW_NAME); exit 1; fi; \
	echo "从 member add 得到 ETCD_INITIAL_CLUSTER: $$INITIAL_CLUSTER"; \
	echo "4) 启动新节点容器 $(NEW_NAME) ..."; \
	docker run -d --name $(NEW_NAME) --network $(NET_NAME) -p $${NEW_PORT}:$${NEW_PORT} \
		-e TZ=Asia/Shanghai \
		-e ALLOW_NONE_AUTHENTICATION=yes \
		-e ETCD_NAME=$(NEW_NAME) \
		-e ETCD_INITIAL_ADVERTISE_PEER_URLS=http://$(NEW_NAME):2380 \
		-e ETCD_LISTEN_PEER_URLS=http://0.0.0.0:2380 \
		-e ETCD_LISTEN_CLIENT_URLS=http://0.0.0.0:$${NEW_PORT} \
		-e ETCD_ADVERTISE_CLIENT_URLS=http://$(NEW_NAME):$${NEW_PORT} \
		-e ETCD_INITIAL_CLUSTER="$$INITIAL_CLUSTER" \
		-e ETCD_INITIAL_CLUSTER_STATE=existing \
		-e ETCD_INITIAL_CLUSTER_TOKEN=$(TOKEN) \
		$(ETCD_IMAGE) >/dev/null; \
	echo "等待几秒，让新节点加入集群..."; sleep 3; \
	echo "5) 打印集群成员列表："; \
	docker run --rm --network $(NET_NAME) --env ETCDCTL_API=3 $(ETCD_IMAGE) \
		etcdctl --endpoints=$(EP) member list

check:
	@echo "列出集群成员（使用 $(EP)）"; \
	docker run --rm --network $(NET_NAME) --env ETCDCTL_API=3 $(ETCD_IMAGE) \
		etcdctl --endpoints=$(EP) member list || echo "member list 调用失败"

leader:
	@echo "查询当前集群 leader ..."
	@docker run --rm --network $(NET_NAME) $(ETCD_IMAGE) sh -c "\
		ENDPOINTS=\$$(etcdctl --endpoints=$(EP) member list | awk -F',' '{gsub(/ /,\"\"); print \$$5}' | paste -sd, -); \
		echo 使用 endpoints: \$$ENDPOINTS; \
		etcdctl --endpoints=\$$ENDPOINTS endpoint status -w table | grep ' true '"

remove:
ifndef NAME
	$(error 请用 make remove NAME=<member_name> 指定要移除的节点名称, 可用 member list 查)
endif
	@echo "移除集群成员 NAME=$(NAME) ..."
	@docker run --rm --network $(NET_NAME) $(ETCD_IMAGE) sh -c "\
		ID=\$$(etcdctl --endpoints=$(EP) member list | grep '$(NAME)' | cut -d',' -f1); \
		if [ -z \"\$$ID\" ]; then \
			echo '错误: 找不到节点 $(NAME)'; exit 1; \
		fi; \
		echo '找到 ID: ' \$$ID; \
		etcdctl --endpoints=$(EP) member remove \$$ID"
	@echo "停止 Docker 容器 $(NAME) ..."
	@docker stop $(NAME) || echo "容器 $(NAME) 已经不在运行"
	@docker rm -f $(NAME) || echo "容器 $(NAME) 已经删除"

get:
	@echo "查询键值: $(KEY)"
	@docker run --rm --network $(NET_NAME) -e EP=$(EP) $(ETCD_IMAGE) sh -c "\
		echo 容器内 EP=\$$EP; \
		ENDPOINTS=\$$(etcdctl --endpoints=\$$EP member list | awk -F',' '{gsub(/ /,\"\"); print \$$5}' | paste -sd, -); \
		echo 使用 endpoints: \$$ENDPOINTS; \
		FIRST_EP=\$$(echo \$$ENDPOINTS | cut -d',' -f1); \
		echo 选用节点: \$$FIRST_EP; \
		if [ -z \"$(KEY)\" ]; then \
			echo '未指定 KEY，列出所有键值...'; \
			etcdctl --endpoints=\$$FIRST_EP get \"\" --prefix --keys-only; \
		else \
			etcdctl --endpoints=\$$FIRST_EP get $(KEY) --print-value-only || echo '键不存在或无法访问'; \
		fi"

put:
ifndef KEY
	$(error 请用 make put KEY=<key_name> VALUE=<value> 来指定键和值)
endif
ifndef VALUE
	$(error 请用 make put KEY=<key_name> VALUE=<value> 来指定键和值)
endif
	@echo "写入键值: $(KEY) = $(VALUE)"
	@docker run --rm --network $(NET_NAME) -e EP=$(EP) $(ETCD_IMAGE) sh -c "\
		echo 容器内 EP=\$$EP; \
		ENDPOINTS=\$$(etcdctl --endpoints=\$$EP member list | awk -F',' '{gsub(/ /,\"\"); print \$$5}' | paste -sd, -); \
		echo 使用 endpoints: \$$ENDPOINTS; \
		FIRST_EP=\$$(echo \$$ENDPOINTS | cut -d',' -f1); \
		echo 选用节点: \$$FIRST_EP; \
		etcdctl --endpoints=\$$FIRST_EP put $(KEY) '$(VALUE)'; \
		echo 写入完成."



stop-all:
	@echo "停止并删除所有名字以 etcd 开头的容器，并断开网络连接..."
	-@docker ps --filter "name=^etcd" --format "{{.Names}}" | while read cname; do \
		echo "断开网络连接 $${cname} ..."; \
		docker network disconnect etcd-net "$${cname}" >/dev/null 2>&1 || true; \
		echo "停止容器 $${cname} ..."; \
		docker stop "$${cname}"; \
		echo "删除容器 $${cname} ..."; \
		docker rm -f "$${cname}"; \
	done
	@docker compose down
