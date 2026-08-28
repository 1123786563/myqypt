# Issue #14 [T13][P2] Product Catalog 浏览 — Implementation Plan

- Issue: https://github.com/1123786563/myqypt/issues/14 （OPEN / ready-for-agent / 0 评论；唯一 blocked_by #3 已 CLOSED 且已并入 main@a86732d/a02c4d6）
- 源计划：`docs/superpowers/plans/2026-08-24-t13-product-catalog.md`（Issue 正文内嵌，作者拆分；本文件为其在本轨道的落位与裁定）
- Branch: `codex/issue-14-t13-product-catalog` ← base `main@a02c4d6`
- Worktree: `.superpowers/worktrees/issue-14-t13-product-catalog`（会话沙箱仅允许写会话工作区——环境裁定见台账）

## Goal

Owner 查看内部策展 Product 及其可用状态。

## Scope（一个垂直切片）

- 新包 `internal/catalog/product-catalog`：`ProductCatalogCommand{TenantID, ProductID, IdempotencyKey}`、`ProductCatalogResult{ResourceID, Outcome}`、端口 `ProductCatalogPort`/`Tx`/`EvidenceSink`、构造 `NewProductCatalogService`、`Execute`（校验先于任何副作用；事务内一次 port 效应 + 一条最小化证据）。
- 具体端口实现（同文件，沿源计划 Step 4 指示）：进程内内部策展目录适配器（ADR-0003 的 Stage-1 形状——仅平台内部团队策展的 Product 条目，每条携带可用状态），幂等键→条目登记表先于可重试工作持久化，重放收敛单效应。
- 旅程三件套：`tests/acceptance/scenarios/t13-product-catalog.yaml` + `t13_product_catalog_driver.go`（新 seam `lighthouse-product-catalog` 经 `platformtest.Register`）+ `t13_product_catalog_test.go`。
- 白盒效应测试 `provider_internal_test.go`（沿 t25 先例）。

## Non-goals

- 不加迁移/schema、不改 `api/openapi`（零契约变更、零 regen）、零 `web/` 改动、不实现公开 marketplace/发布流（ADR-0003 明确开放市场推迟）、不做角色门控（T06 capabilities 在未合并分支；本切片命令形状沿源计划无 viewer/role 字段）、不触 tenancy/identity 既有代码、不做 Catalog 事件/CDN/快照（F14–F17 后续票）。

## Design rulings

1. **包位置与新域**：`internal/catalog/product-catalog` 为应用层新域包，仅依赖标准库（依赖方向单向，policy-check 架构策略必须通过）。命名沿源计划（`internal/catalog` 顶层新目录，先例 = t25 的 `internal/security`）。
2. **最高可行接缝 = platformtest 旅程（进程内）**：本票无 HTTP 契约（源计划自证：Step 7 直接 `platformtest.Run` 不起栈）——旅程驱动真实服务+具体端口+记录证据 sink，以 `lighthouse-product-catalog` seam 注册；不起 compose 栈。
3. **内部策展目录（ADR-0003 可执行化）**：具体端口持有平台内部策展的封闭条目集（测试用稳定非敏感标识）；条目 = ProductID + 可用状态。**可用状态封闭词表** `availability ∈ {available, unavailable}`；目录无任何外部发布路径（append-only 策展集合，代码内字面量构建——"内部策展"的形状强制）。未知/未策展 ProductID → 端口分类拒绝 `ErrProductNotCurated`（denied 类）。
4. **Owner 语义边界（诚实落位）**：源计划命令形状 `{TenantID, ProductID, IdempotencyKey}` 无 viewer/role 字段 → 本切片证明「租户作用域内的内部策展目录读取 + 可用状态」这一可观察能力；角色门控（Owner/Admin 差异视图）属 T06 capabilities 集成，非本票范围（记录为移交项，不虚构字段）。
5. **幂等语义**：登记表幂等键→访问条目先于可重试工作持久化；首投 `accepted`（一效应、一证据行），重投 `duplicate`（零新效应）；异键=新效应。端口失败（`ErrCatalogUnavailable` 分类）后零半成品，重试收敛单效应。
6. **事务边界**：`Tx.Run` 包裹 port 效应 + 证据写入；任一失败整体回滚语义（进程内 Tx = 直接执行；接口形状为将来真实 DB 事务预留，沿源计划）。
7. **证据最小化**：`EvidenceSink.Record(ctx, idempotencyKey, resourceID, outcome)` 三字符串；旅程断言证据内容零秘密材料/零客户内容。
8. **错误词表**：`ErrTenantRequired`/`ErrIdempotencyKeyRequired`/`ErrProductRequired`（先于端口）；`ErrProductNotCurated`（denied，来自端口）/`ErrCatalogUnavailable`（可重试，来自端口）。sentinel 风格同构 tenancy/secret-reference 包先例。
9. **旅程断言集（YAML name ↔ driver 双锚，harness 强制对账）**：`reject_missing_tenant`、`reject_missing_idempotency_key`、`reject_missing_product`、`curated_product_availability_visible`（available 条目带状态可见）、`unavailable_product_status_visible`（unavailable 条目同样带状态可见——「及其可用状态」的第二半）、`unknown_product_denied`、`replay_converges_single_effect`、`port_failure_no_partial_then_retry_converges`、`evidence_content_minimized`（9 条）。
10. **零 schema/契约变更**：无迁移文件、无 openapi 改动、go.mod 零触碰。

## Task breakdown

- **Task 0（controller）**：本计划，提交 `docs(plan): add issue 14 t13 implementation plan`。
- **Task 1（实施者——本会话为 controller fallback，独立性披露见台账；仍守 RED→green、恰一提交纪律）**：focused 契约测试先红（`service_test.go` 引用不存在词汇 build fail，RED 证据落 `artifacts/evidence/task1/`）→ 实现 `service.go`（含具体端口）→ focused 全绿 → 旅程三件套 → 域回归 → 13 门禁。
- **Task 1 双审**：规格符合性 + 代码质量（独立 subagent；本会话不可用→controller 双轴报告 + 醒目披露）。
- **终审**：最强可用模型全分支审查（本会话=controller 自身，披露）。

## Acceptance matrix（13 门禁；审查/终审逐条独立重跑）

环境：`GOTOOLCHAIN=local`、`/Users/wuyongjun/.local/go1.26.7/bin/go`、`GOPROXY=https://goproxy.cn,direct GOSUMDB=off`、禁 `env -u`、`TestPlatformAPIProcess` 在场时 `-p 1`、WeKnora 端口勿触、临时 PG 用 55xxx 毕拆。**会话沙箱适配**：worktree 位于 `.superpowers/worktrees/`（会话工作区内）；全部 go 命令 `GOCACHE=/tmp/t13-gocache`；需树写的验证在 worktree 内直接可行。

1. `go test ./internal/catalog/product-catalog -count=1` — focused 全绿。
2. `go test -race -count=1 ./internal/catalog/product-catalog` — 无竞争。
3. `go test ./tests/acceptance -run TestT13ProductCatalog -count=1 -v` — 旅程 PASS（无栈门控、不 skip），evidence JSON 落 `artifacts/evidence/t13-product-catalog/`。
4. `go test ./... -count=1 -p 1`（无 DB env） — 全仓绿、T13 旅程运行不 skip。
5. `go vet ./...`、`gofmt -l .`（除 web/）空、`go build ./...`、`go mod tidy -diff` 空。
6. `make generate-check` — 零漂移（本切片零 codegen 触碰的自证）。
7. `make policy-check` — 新包过依赖策略。
8. `bash scripts/verify-foundation.sh`（TEST_DATABASE_URL=临时 PG 55487）— 七相位全 PASS。
9. 旅程证据审计 — JSON passed=true、9/9 断言、details 双脱敏、伪造敏感材料零命中。
10. 提交卫生 — 恰 2 提交（plan+slice）、树净、`git diff --check` 干净。
11. RED 独立复现 — /tmp scratch：base + 仅新测试 → 恰红于新词汇边界；对照基线全绿。
12. 秘密扫描 — 全新增 diff 扫 token/secret/password 模式：仅命中文档散文与公开测试标识符。
13. 零回归 — gates 4/8 + diff 审计（不触任何既有文件；唯一共享接缝 = tests/acceptance 新增文件，无修改既有文件）。

## Global constraints（Issue #14 原文，逐条守）

- Stage 1 规模包络/单 Region；Tenant 硬边界（命令携带 TenantID，校验先于副作用）；billing 1:1；Product Domain Objects Product 拥有（本切片仅读取策展目录元数据，不触 Product 内部对象）；**秘密/原始 prompt/文档体/原始支付载荷/敏感个人信息不入 logs/traces/metrics/Audit/Usage/fixtures/evidence**（ruling 7 可执行化）；compose 限开发/CI/受控 beta；99.9%/RPO/RTO 目标（本切片不涉）；聚焦单测不替代具名验收接缝（ruling 2 旅程=具名接缝）；依赖图完备（#3 CLOSED 亲验）；域词表与已接受 ADR 边界保持（ADR-0003 内部策展目录即本票所立形状）。
