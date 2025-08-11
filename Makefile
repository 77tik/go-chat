# ========================
# 统一配置
# ========================
PROJECT_NAME := gochat
SERVER_BIN := gochat.bin
MODULES := logic connect_tcp task api site
LOG_DIR := ./log
# 确保使用正确的日志目录

# 端口配置
LOGIC_PORT := 7000
SITE_PORT := 8080
API_PORT := 7070
TCP_CONNECT_PORT := 7001
WEBSOCKET_CONNECT_PORT := 7002
TASK_WAIT_TIME := 15
# ========================
# 帮助系统
# ========================
.PHONY: help
help: ## 显示帮助信息
	@echo "GoChat Project Management"
	@echo "========================="
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-25s\033[0m %s\n", $$1, $$2}'
	@echo "\nRecommended workflow:"
	@echo "  make all       # Full build and start"
	@echo "  make run       # Start system (after build)"
	@echo "  make stop      # Stop entire system"
	@echo "\nAdjust task wait time if needed:"
	@echo "  make set-wait-longer   # Set task wait to 30s"
	@echo "  make set-wait-default  # Reset task wait to 15s"
	@echo "\nMiddleware commands:"
	@echo "  make mw-redis-cli      # Access Redis CLI"
	@echo "  make mw-etcd-getall    # View all etcd keys"
	@echo "  make mw-start-visuals   # Start monitoring tools"

# ========================
# 容器进入与诊断 (添加前缀避免冲突)
# ========================
.PHONY: mw-redis-cli
mw-redis-cli: ## 进入 Redis 容器并启动命令行
	docker exec -it yoyichat-redis redis-cli

.PHONY: mw-redis-shell
mw-redis-shell: ## 进入 Redis 容器系统 Shell
	docker exec -it yoyichat-redis /bin/sh

.PHONY: mw-etcd-shell
mw-etcd-shell: ## 进入 etcd 容器系统 Shell
	docker exec -it yoyichat-etcd /bin/sh

.PHONY: mw-etcd-getall
mw-etcd-getall: ## 查看 etcd 所有键值
	docker exec yoyichat-etcd sh -c 'ETCDCTL_API=3 etcdctl get --prefix ""'

# ========================
# 数据查看与导出 (添加前缀)
# ========================
.PHONY: mw-redis-keys
mw-redis-keys: ## 列出 Redis 所有键
	@echo "Keys in yoyichat-redis:"
	@docker exec yoyichat-redis redis-cli KEYS '*'

.PHONY: mw-export-redis
mw-export-redis: ## 导出 Redis 数据到宿主机
	mkdir -p ./exports
	docker exec yoyichat-redis redis-cli --raw SAVE
	docker cp yoyichat-redis:/data/dump.rdb ./exports/redis_dump_$(shell date +%Y%m%d).rdb
	@echo "Redis dump exported to: exports/redis_dump_$(shell date +%Y%m%d).rdb"

.PHONY: mw-export-etcd
mw-export-etcd: ## 导出 etcd 数据到宿主机
	mkdir -p ./exports
	docker exec yoyichat-etcd sh -c 'ETCDCTL_API=3 etcdctl snapshot save /tmp/snapshot.db'
	docker cp yoyichat-etcd:/tmp/snapshot.db ./exports/etcd_snapshot_$(shell date +%Y%m%d).db
	@echo "Etcd snapshot exported to: exports/etcd_snapshot_$(shell date +%Y%m%d).db"

# ========================
# 可视化工具管理 (添加前缀)
# ========================
.PHONY: mw-start-visuals
mw-start-visuals: ## 启动可视化工具
	@echo "Starting visualization tools..."
	docker run -d --name redisinsight -p 8001:8001 redislabs/redisinsight:latest
	docker run -d --name etcd-ui -p 8080:8080 appscode/etcd-ui:v0.7.0
	@echo "RedisInsight: http://localhost:8001"
	@echo "Etcd UI: http://localhost:8080/?endpoints=127.0.0.1:2379"

.PHONY: mw-stop-visuals
mw-stop-visuals: ## 停止可视化工具
	docker stop redisinsight etcd-ui || true
	docker rm redisinsight etcd-ui || true

# ========================
# 主控制流程
# ========================
.PHONY: all
all: prerequisites build run ## 完整流程：准备 → 编译 → 启动系统

.PHONY: run
run: start-middleware wait-middleware start-services ## 启动全套系统（中间件 + 应用）

.PHONY: stop
stop: stop-app stop-middleware ## 停止全套系统（应用 + 中间件）

# ========================
# 严格顺序的服务启动
# ========================
.PHONY: start-services
start-services: create-log-dir ## 按严格顺序启动所有服务模块
	@echo "=== Starting services in strict order ==="
	$(MAKE) start-logic
	$(MAKE) start-connect
	$(MAKE) wait-for-task-deps
	$(MAKE) start-task
	$(MAKE) start-api
	$(MAKE) start-site

# 确保日志目录存在
.PHONY: create-log-dir
create-log-dir:
	@mkdir -p ${LOG_DIR}
	@chmod 755 ${LOG_DIR}

# 模块独立控制目标
.PHONY: start-logic
start-logic: ## 启动 logic 层
	@echo "Starting logic layer (port ${LOGIC_PORT})..."
	./${SERVER_BIN} -module logic > "${LOG_DIR}/logic.log" 2>&1 &
	@sleep 2  # 基本初始化等待

.PHONY: start-connect
start-connect: ## 启动 connect 层（TCP + WebSocket）
	@echo "Starting TCP connect (port ${TCP_CONNECT_PORT})..."
	./${SERVER_BIN} -module connect_tcp > "${LOG_DIR}/connect_tcp.log" 2>&1 &

	@echo "Starting WebSocket connect (port ${WEBSOCKET_CONNECT_PORT})..."
	./${SERVER_BIN} -module connect_websocket > "${LOG_DIR}/connect_ws.log" 2>&1 &

	@echo "Waiting 5s for connect layers to initialize..."
	@sleep 5

.PHONY: wait-for-task-deps
wait-for-task-deps: ## 等待前置服务就绪
	@echo "Waiting ${TASK_WAIT_TIME} seconds for dependencies before starting task..."
	@echo "This ensures logic and connect layers are fully ready"
	@sleep ${TASK_WAIT_TIME}
	@echo "Proceeding to start task layer..."

.PHONY: start-task
start-task: ## 启动 task 层 (修复路径错误)
	@echo "Starting task layer ..."
	./${SERVER_BIN} -module task > "${LOG_DIR}/task.log" 2>&1 &
	@sleep 3  # 基本初始化等待

.PHONY: start-api
start-api: ## 启动 API 层
	@echo "Starting API layer (port ${API_PORT})..."
	./${SERVER_BIN} -module api > "${LOG_DIR}/api.log" 2>&1 &
	@sleep 2

.PHONY: start-site
start-site: ## 启动站点
	@echo "Starting chat site (port ${SITE_PORT})..."
	./${SERVER_BIN} -module site > "${LOG_DIR}/site.log" 2>&1 &
	@sleep 1
	@echo "All services started! Site should be accessible now."

# ========================
# 中间件管理
# ========================
.PHONY: prerequisites
prerequisites: ## 确保端口可用
	@echo "Checking required ports..."
	@for port in ${LOGIC_PORT} ${SITE_PORT} ${API_PORT} ${TCP_CONNECT_PORT} ${WEBSOCKET_CONNECT_PORT}; do \
		if lsof -i :$$port >/dev/null 2>&1; then \
			echo "ERROR: Port $$port is already in use!"; \
			exit 1; \
		else \
			echo "Port $$port: AVAILABLE"; \
		fi \
	done

.PHONY: start-middleware
start-middleware: ## 启动中间件 (etcd + redis)
	@echo "Starting middleware containers..."
	docker-compose up -d
	@echo "Middleware containers started"

.PHONY: stop-middleware
stop-middleware: ## 停止中间件
	@echo "Stopping middleware containers..."
	docker-compose down
	@echo "Middleware containers stopped"

.PHONY: wait-middleware
wait-middleware: ## 等待中间件就绪 (使用容器名)
	@echo "Waiting for middleware to initialize..."
	@echo "Waiting for Redis to be ready..."
	@until docker exec yoyichat-redis redis-cli ping | grep -q PONG; do sleep 1; done
	@echo "Redis ready"

	@echo "Waiting for etcd to be ready..."
	@until docker exec yoyichat-etcd etcdctl endpoint health; do sleep 1; done
	@echo "etcd ready"

	@echo "Middleware fully initialized"

# ========================
# 构建与控制
# ========================
.PHONY: build
build: ## 编译项目
	@echo "Building application..."
	go build -o ${SERVER_BIN} -tags=etcd main.go
	@echo "Build complete: ${SERVER_BIN}"

.PHONY: stop-app
stop-app: ## 停止所有应用模块
	@echo "Stopping all application modules..."
	-pkill -f ${SERVER_BIN} || true
	@sleep 1
	@echo "Application modules stopped"

# ========================
# 诊断与监控
# ========================
.PHONY: status
status: ## 查看系统状态
	@echo "=== Middleware Containers ==="
	@docker-compose ps --services | while read service; do \
		if docker-compose ps -q $$service >/dev/null 2>&1; then \
			status=$$(docker-compose ps $$service | awk 'NR==3 {print $$4}'); \
			printf "  %-15s %s\n" $$service "$$status"; \
		else \
			printf "  %-15s %s\n" $$service "not running"; \
		fi \
	done

	@echo "\n=== Application Processes ==="
	@for module in ${MODULES}; do \
		if pgrep -f "./${SERVER_BIN} -module $$module" > /dev/null; then \
			printf "  %-15s %s\n" "$$module" "running"; \
		else \
			printf "  %-15s %s\n" "$$module" "stopped"; \
		fi \
	done

	@echo "\n=== Port Usage ==="
	@for port in ${LOGIC_PORT} ${SITE_PORT} ${API_PORT} ${TCP_CONNECT_PORT} ${WEBSOCKET_CONNECT_PORT}; do \
		if lsof -i :$$port &>/dev/null; then \
			proc=$$(lsof -i :$$port | awk 'NR==2 {print $1}'); \
			pid=$$(lsof -i :$$port | awk 'NR==2 {print $2}'); \
			printf "  %-15s %s (by %s PID:%s)\n" "Port $$port" "IN USE" "$$proc" "$$pid"; \
		else \
			printf "  %-15s %s\n" "Port $$port" "AVAILABLE"; \
		fi \
	done

# ========================
# 等待时间调整
# ========================
.PHONY: set-wait-longer
set-wait-longer: ## 增加task等待时间到30秒
	$(eval TASK_WAIT_TIME=30)
	@echo "Task wait time set to ${TASK_WAIT_TIME} seconds"

.PHONY: set-wait-default
set-wait-default: ## 重置task等待时间为默认(15秒)
	$(eval TASK_WAIT_TIME=15)
	@echo "Task wait time reset to default (${TASK_WAIT_TIME} seconds)"

