#!/usr/bin/env ruby
# frozen_string_literal: true

require "json"
require "open3"

REPO = "1123786563/myqypt"
PARENT = "87"

SUBISSUES = [
  ["T86.1", "Privacy、未成年人、Export、Erasure 与 Operator Access Dossier", [], "确认中国大陆隐私、未成年人保护、数据删除、Tenant Export、Operator Access、事件通知与法定保留边界。"],
  ["T86.2", "Tax 与 Electronic Invoice Dossier", [64], "由可承担责任的财税审核者确认税率、发票类型、开具时点、作废、红冲与法定保存期。"],
  ["T86.3", "WeChat Pay 与 Alipay Merchant Dossier", [58, 59], "确认微信支付和支付宝的商户资质、证书、回调、主动查询、退款、对账与生产验收要求。"],
  ["T86.4", "Product Version License Dossier", [67], "汇总每个 Product Version 的 License Report、商业托管权与法律批准记录。"],
  ["T86.5", "Model Provider Processing Terms Dossier", [16, 28], "确认每个获批模型 Provider 的 Region、Retention、Training Use、Subprocessor 与商业 API 权利。"],
  ["T86.6", "Mainland China Cloud Capability Dossier", [82, 83, 84, 85], "确认选定云厂商在中国大陆 Region 的 KMS、Secret、PostgreSQL、Kafka、ClickHouse、Registry、多 AZ 与备份能力。"],
  ["T86.7", "Nacos Version 与 Production Baseline Dossier", [75], "在实施时重新确认 Nacos 稳定版本、Java 要求、鉴权、三节点、外置 PostgreSQL、备份与恢复边界。"],
  ["T86.8", "Valkey 与 OpenMeter Compatibility Dossier", [56], "收集 Valkey 对当前 OpenMeter Redis 命令面、TLS、持久化、并发去重、Failover 与恢复的可复现证据。"],
  ["T86.9", "WeKnora Shared Security Dossier", [39, 69, 71, 73], "汇总 WeKnora Shared TenantScope、向量、任务、缓存、Upgrade、Export 与 Erasure 的攻击和恢复证据。"],
  ["T86.10", "OpenMeter Commerce Chain Dossier", [63, 65], "汇总微信与支付宝从 created、awaiting_payment、paid 到 fulfilled、Credit、Refund 与 Reconciliation 的 Provider sandbox 和真实商户证据。"]
].freeze

def run!(*args)
  out, err, status = Open3.capture3(*args)
  raise "#{args.join(' ')} failed: #{err}" unless status.success?
  out.strip
end

existing = JSON.parse(run!("gh", "issue", "list", "--repo", REPO, "--state", "all", "--limit", "200", "--json", "number,title"))

SUBISSUES.each do |ticket, title, blockers, goal|
  full_title = "[#{ticket}][P23] #{title}"
  match = existing.find { |issue| issue.fetch("title") == full_title }
  if match
    puts "existing ##{match.fetch('number')} #{full_title}"
    next
  end

  body = <<~MARKDOWN
    ## Parent

    #87

    ## Parallel batch

    P23-evidence. These dossier sub-Issues may run in parallel after their own blockers are complete.

    ## What to build

    #{goal}

    ## Acceptance criteria

    - [ ] Evidence cites current primary or accountable professional sources with retrieval date, version or effective date, jurisdiction, and scope.
    - [ ] Unknowns, contradictions, expiry dates, owner, renewal trigger, and explicit paid-launch consequence are recorded without inventing a legal or Provider conclusion.
    - [ ] The dossier contains no Secret, raw payment payload, Prompt, document body, or sensitive personal information.
    - [ ] An accountable reviewer records approve or block with identity, timestamp, rationale, and evidence digest before the parent Issue can close.

    ## Blocked by

    #{blockers.empty? ? '- None — evidence collection can start immediately' : blockers.map { |n| "- ##{n}" }.join("\n")}
  MARKDOWN

  args = ["gh", "issue", "create", "--repo", REPO, "--title", full_title, "--body", body,
          "--label", "ready-for-human", "--parent", PARENT]
  blockers.each { |number| args.concat(["--blocked-by", number.to_s]) }
  url = run!(*args)
  puts "created #{url}"
end
