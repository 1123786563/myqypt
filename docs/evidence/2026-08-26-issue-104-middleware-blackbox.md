# Issue #104 中间件黑盒证据 — 2026-08-26

- **Worktree:** `/Users/wuyongjun/trea/myqypt-worktrees/issue-104-f04-http-security-observability`（branch `codex/issue-104-f04-http-security-observability`）
- **被测进程:** `platform-api serve`（真实二进制，`go build -o /tmp/issue104-impl2-blackbox/platform-api ./cmd/platform-api`）
- **环境:** `PLATFORM_API_ADDR=127.0.0.1:18081`、`PLATFORM_API_ALLOWED_ORIGINS=http://allowed.example`；`DATABASE_URL` 与 `OTEL_EXPORTER_OTLP_ENDPOINT` 均未设置（无 DB、无 OTLP 导出，网络空闲路径）；stdout+stderr 重定向 `/tmp/issue104-impl2-blackbox/server-stdout.log`
- **文件名说明:** 计划矩阵第 12 行写作 `2026-08-25-issue-104-middleware-blackbox.md`；实施指令（controller）指定 `2026-08-26-…`，按实施指令执行并在此记录偏差。
- **执行脚本瑕疵（诚实披露）:** 首轮驱动脚本末段的 echo 引号错误（`unexpected EOF while looking for matching '"'`）导致 (e) 之后的全量 marker 扫描与 SIGTERM 收尾未随脚本执行；(a)-(e) 五项探针均在脚本内完整执行且输出真实。收尾两步（全量日志 marker 扫描、SIGTERM 优雅退出）随后在同一环境下补跑并逐字记录如下。进程退出与端口释放均已核实，零残留。

## (a) `/livez` 200 + X-Request-ID 回显非空 + 四安全头

```
$ curl -sS -D - http://127.0.0.1:18081/livez
HTTP/1.1 200 OK
Content-Security-Policy: default-src 'none'; frame-ancestors 'none'
Content-Type: application/json
Referrer-Policy: no-referrer
Strict-Transport-Security: max-age=31536000; includeSubDomains
X-Content-Type-Options: nosniff
X-Request-Id: a883159b419b3e04
Date: Tue, 25 Aug 2026 16:28:49 GMT
Content-Length: 18

{"status":"alive"}
```

判定：✅ 200；`X-Request-Id: a883159b419b3e04` 非空回显；HSTS / CSP / nosniff / Referrer-Policy 四头齐全。

## (b) 未知路径 404 problem + trace_id = 回显 request ID

```
$ curl -sS -D - -H "X-Request-ID: blackbox-probe-b" http://127.0.0.1:18081/no/such/path
HTTP/1.1 404 Not Found
Content-Security-Policy: default-src 'none'; frame-ancestors 'none'
Content-Type: application/problem+json
Referrer-Policy: no-referrer
Strict-Transport-Security: max-age=31536000; includeSubDomains
X-Content-Type-Options: nosniff
X-Request-Id: blackbox-probe-b
Date: Tue, 25 Aug 2026 16:28:49 GMT
Content-Length: 162

{"type":"https://api.myqypt.dev/problems/not_found","title":"The requested resource was not found.","status":404,"code":"not_found","trace_id":"blackbox-probe-b"}
```

判定：✅ 404 `application/problem+json`；problem 的 `trace_id` 逐字等于入站回显的 `blackbox-probe-b`。

## (c) 非法入站 X-Request-ID（200 字符 'x'）被替换

```
$ curl -sS -o /dev/null -D - -H "X-Request-ID: <200-char value of x>" http://127.0.0.1:18081/livez
X-Request-Id: d43120ea0f62f0c2
```

判定：✅ 200 字符入站 ID 超出 `^[A-Za-z0-9-_]{1,64}$`，响应回显替换为新生成的 16-hex ID。

## (d) OPTIONS preflight：允许 origin 204 + ACAO 回显；拒绝 origin 无 ACAO

```
$ curl -sS -o /dev/null -D - -X OPTIONS -H "Origin: http://allowed.example" -H "Access-Control-Request-Method: POST" http://127.0.0.1:18081/livez
HTTP/1.1 204 No Content
Access-Control-Allow-Headers: Content-Type, X-Request-ID
Access-Control-Allow-Methods: GET, POST, PUT, PATCH, DELETE, OPTIONS
Access-Control-Allow-Origin: http://allowed.example
Allow: GET
Content-Security-Policy: default-src 'none'; frame-ancestors 'none'
Referrer-Policy: no-referrer
Strict-Transport-Security: max-age=31536000; includeSubDomains
Vary: Origin
X-Content-Type-Options: nosniff
X-Request-Id: 5994e09878d40fff
Date: Tue, 25 Aug 2026 16:28:49 GMT
```

判定：✅ 允许 origin preflight 短路 204，ACAO 精确回显 `http://allowed.example`，附 Methods/Headers/Vary。此请求在 Security 层被短路，位于其后的 access log 中间件不执行，故访问日志无此行——这是 Task 1 已提交的短路设计（裁定 6），非本次缺陷。

```
$ curl -sS -o /dev/null -D - -X OPTIONS -H "Origin: http://evil.example" -H "Access-Control-Request-Method: POST" http://127.0.0.1:18081/livez
HTTP/1.1 405 Method Not Allowed
Allow: GET
Content-Security-Policy: default-src 'none'; frame-ancestors 'none'
Content-Type: application/problem+json
Referrer-Policy: no-referrer
Strict-Transport-Security: max-age=31536000; includeSubDomains
Vary: Origin
X-Content-Type-Options: nosniff
X-Request-Id: f2ccc251a3237cbf
Date: Tue, 25 Aug 2026 16:28:49 GMT
Content-Length: 192
```

判定：✅ 拒绝 origin 无任何 `Access-Control-Allow-*` 头（仅 `Vary: Origin`）；OPTIONS 非 /livez 注册方法 → 405 problem，安全头仍在。

## (e) Authorization/Cookie marker 零出现（响应 + stdout JSON 访问日志）

```
$ curl -sS -D - -H "X-Request-ID: blackbox-probe-e" -H "Authorization: Bearer marker-auth-XYZ" -H "Cookie: session=marker-cookie-XYZ" http://127.0.0.1:18081/livez
HTTP/1.1 200 OK
Content-Security-Policy: default-src 'none'; frame-ancestors 'none'
Content-Type: application/json
Referrer-Policy: no-referrer
Strict-Transport-Security: max-age=31536000; includeSubDomains
X-Content-Type-Options: nosniff
X-Request-Id: blackbox-probe-e
Date: Tue, 25 Aug 2026 16:28:49 GMT
Content-Length: 18

{"status":"alive"}
```

响应侧 marker 计数（grep exit 1 = 零匹配）：

```
$ grep -c 'marker-auth-XYZ\|marker-cookie-XYZ' <response>
0
```

服务器 stdout 访问日志（含本次请求的行，`request_id` 回显 `blackbox-probe-e`，`trace_id` 为 OTel 32-hex）：

```
{"time":"2026-08-26T00:28:49.172202+08:00","level":"INFO","msg":"http request","method":"GET","path":"/livez","status":200,"duration_ms":0,"request_id":"blackbox-probe-e","trace_id":"bda56645d8b000158847c0ebdac45200"}
```

全量日志 marker 扫描（脚本收尾补跑）：

```
$ grep -c 'marker-auth-XYZ\|marker-cookie-XYZ' /tmp/issue104-impl2-blackbox/server-stdout.log
0
grep_exit=1
```

判定：✅ 响应与 stdout 访问日志 marker 零出现；日志字段恰为 method/path/status/duration_ms/request_id/trace_id 白名单。

## 关停：SIGTERM → 优雅退出（组合根 Shutdown 路径）

```
$ kill -TERM <pid>
server exit code=0
lsof exit 1 -> port 18081 free
```

判定：✅ SIGTERM 后进程在超时内优雅退出（exit 0，含 runtime.Serve 排水与 observability Shutdown），端口释放。

## 清理

临时目录 `/tmp/issue104-impl2-blackbox/`（二进制、日志、脚本）用后删除；无监听端口、无后台进程残留。
