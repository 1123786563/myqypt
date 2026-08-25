# Issue #103 Compose 冒烟证据 — 2026-08-25

- **Worktree:** `/Users/wuyongjun/trea/myqypt-worktrees/issue-103-f03-postgres-migrations-readiness`（branch `codex/issue-103-f03-postgres-migrations-readiness`）
- **Stack:** `deploy/compose.yaml`（postgres:18 + platform-api on golang:1.26.7，migrate-then-serve）
- **Host ports:** postgres `127.0.0.1:15532`，API `127.0.0.1:18080`
- **Docker:** 29.4.0，Compose v5.1.2

## 首次起栈的一次失败尝试（诚实披露）

第一次 `docker compose -f deploy/compose.yaml up -d --wait` 以 `dependency failed to start: container deploy-postgres-1 exited (1)` 失败。原因：postgres 18 镜像更改了数据目录约定——卷应挂在 `/var/lib/postgresql`（由镜像自管版本化子目录），直接挂 `/var/lib/postgresql/data` 会被入口脚本以 "there appears to be PostgreSQL data in: /var/lib/postgresql/data (unused mount/volume)" 拒绝（详见 docker-library/postgres#1259、#37）。修复：compose 卷路径改为 `platform-postgres-data:/var/lib/postgresql`，`down -v` 清理后重跑成功。此为 Task 1 集成测试未暴露的问题（其容器不挂数据卷）。

## 冒烟命令与响应（逐条原样记录）

### 1. 起栈并等待健康

```
$ docker compose -f deploy/compose.yaml up -d --wait
 Container deploy-postgres-1 Healthy
 Container deploy-platform-api-1 Healthy
UP_WAIT_EXIT=0
```

### 2. 就绪与存活端点（栈健康态）

```
$ curl -sS -i http://127.0.0.1:18080/livez
HTTP/1.1 200 OK
Content-Type: application/json

{"status":"alive"}

$ curl -sS -i http://127.0.0.1:18080/readyz
HTTP/1.1 200 OK
Content-Type: application/json

{"checks":{"database":"ok"}}
```

### 3. 停库 → readiness 过渡为 503（fail closed）

```
$ docker compose -f deploy/compose.yaml stop postgres
 Container deploy-postgres-1 Stopped
STOP_EXIT=0
$ # 轮询 /readyz
poll 1: /readyz=503 body={"checks":{"database":"failed"}}
TRANSITION_TO_503_AT_POLL=1
```

（响应体仅含检查名与状态，不含错误文本/DSN/主机名。）

### 4. 重启库 → readiness 恢复 200（无需重启进程）

```
$ docker compose -f deploy/compose.yaml start postgres
 Container deploy-postgres-1 Started
START_EXIT=0
$ # 轮询 /readyz
poll 1: /readyz=503 body={"checks":{"database":"failed"}}
poll 2: /readyz=200 body={"checks":{"database":"ok"}}
RECOVERED_TO_200_AT_POLL=2

$ curl -sS -i http://127.0.0.1:18080/livez   # 全程存活
HTTP/1.1 200 OK
{"status":"alive"}
```

### 5. 拆栈并核实无残留

```
$ docker compose -f deploy/compose.yaml down -v
 Container deploy-platform-api-1 Removed
 Container deploy-postgres-1 Removed
 Volume deploy_platform-postgres-data Removed
 Volume deploy_platform-api-go-modules Removed
 Network deploy_default Removed
DOWN_EXIT=0

$ docker ps -a --filter name=deploy- --format '{{.Names}} {{.Status}}'      # 空
$ docker volume ls --filter label=com.docker.compose.project=deploy        # 空
$ docker network ls --filter label=com.docker.compose.project=deploy       # 空
```

## 结论

- migrate-then-serve 起栈后 `/livez`=200、`/readyz`=200；
- 数据库停止后 `/readyz` 立即 503（`{"checks":{"database":"failed"}}`），`/livez` 持续 200；
- 数据库恢复后 `/readyz` 约 4 秒内回到 200（readiness transition，无进程重启）；
- 冒烟后无残留容器/卷/网络。
