# myqypt

面向中国大陆大众用户和小型企业的公共多租户 AI Application SaaS Platform：围绕独立 AI Product 提供统一身份、目录、订阅、预付用量、授权与生命周期管理。WeKnora 是 Lighthouse Product。

当前状态：设计文档 + 实施计划语料 + 管理/Portal 基础代码（Go + React）。架构方向与硬约束见基线，持久决策见 ADR。

## 仓库地图

| 路径 | 内容 |
| --- | --- |
| `CONTEXT.md` | 领域词汇表（唯一权威术语来源） |
| `docs/architecture/architecture-baseline-risk-assessment-v1.1.md` | 架构基线与风险评估（Stage-1 约束、领域模型、P0 风险） |
| `docs/architecture/ADR-INDEX.md` | ADR 索引（0057 已裁决 13 项不可豁免 Production Gate） |
| `docs/architecture/external-confirmations.md` | 外部确认登记表（封闭测试/付费上线前置） |
| `docs/architecture/compliance-retention-policy.md` | 合规保留期政策（draft，年限待法务确认） |
| `docs/adr/` | 架构决策记录 |
| `docs/agents/` | agent 约定（issue tracker、triage 标签、领域文档纪律） |
| `docs/superpowers/plans/` | 实施计划：stage-1 索引（T 系列）、admin-foundation 索引（F 系列）、portal-ui 索引（U 系列）、P0 Gate 模板 |
| `tools/plans/` | 计划自动化（状态见其 README；sync 已重写，generator 已归档） |

## Issue 跟踪

GitHub `1123786563/myqypt`（**所有 gh 命令必须显式 `--repo 1123786563/myqypt`**）。

- 系列：T01–T88（含 T86.x 外部确认 dossier、T01.x 基础）、F01–F21（工程基础）、U00–U08（Portal 商业 UI，#122–#130）。
- 标签：`needs-triage` / `needs-info` / `ready-for-agent` / `ready-for-human` / `wontfix`。
- 计划文件与 issue 正文经 `tools/plans/sync_stage1_plans_to_github.rb` 同步（先 `--dry-run`）。

## Agent 工作约定

1. 先读 `AGENTS.md`、`CONTEXT.md` 与相关 ADR；术语以 CONTEXT.md 为准，不得与已接受 ADR 冲突。
2. 只执行 blocker 已清空且计划存在的 issue；每个计划是单一垂直切片，按 TDD 步骤逐项推进。
3. P0 Gate 类 ticket 遵循 `docs/superpowers/plans/2026-08-25-p0-gate-template.md`（case 矩阵 + 机械判据 + 证据 schema + 四方批准；禁止目标句验收）。
4. 修改计划文件后运行 sync（`--dry-run` 先行）保持 issue 正文一致；不得手改 issue 的 Implementation plan 段（会被覆盖），issue 描述其余部分是持久规范。
5. 验收证据进 `docs/evidence/`，不进聊天或未跟踪文件。
