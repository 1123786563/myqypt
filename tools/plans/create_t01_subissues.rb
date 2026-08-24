#!/usr/bin/env ruby
# frozen_string_literal: true

require "json"
require "open3"

REPO = "1123786563/myqypt"

def run!(*args)
  out, err, status = Open3.capture3(*args)
  raise "#{args.join(' ')} failed: #{err}" unless status.success?
  out.strip
end

existing = JSON.parse(run!("gh", "issue", "list", "--repo", REPO, "--state", "all", "--limit", "200", "--json", "number,title"))

items = [
  {
    title: "[T01.1][P0] Minimal Platform Scaffold 与 Test Harness",
    goal: "建立 Go Platform API、PostgreSQL migration、Docker Compose 开发栈，以及 acceptance、conformance、production-gates 共用的可执行证据 harness。",
    blockers: []
  },
  {
    title: "[T01.2][P0] Keycloak Verified Subject 与 Identity Binding",
    goal: "只接受经 Keycloak OIDC 验证的 issuer + subject，在 Platform PostgreSQL 幂等建立 User 与 Identity Binding，且不使用 email、phone 或 username 作为身份键。",
    blockers: :scaffold
  }
]

created = {}
items.each_with_index do |item, index|
  match = existing.find { |issue| issue.fetch("title") == item.fetch(:title) }
  if match
    created[index] = match.fetch("number")
    puts "existing ##{match.fetch('number')} #{item.fetch(:title)}"
    next
  end

  blockers = item.fetch(:blockers) == :scaffold ? [created.fetch(0)] : []
  body = <<~MARKDOWN
    ## Parent

    #2

    ## Parallel batch

    P0-foundation. T01.2 starts only after T01.1 is complete.

    ## What to build

    #{item.fetch(:goal)}

    ## Acceptance criteria

    - [ ] The normal path is proven by an executable automated test at the highest practical seam.
    - [ ] Invalid identity, dependency failure, retry, and duplicate delivery have deterministic outcomes without duplicate business effects.
    - [ ] Verification evidence names the source revision and dependency versions and contains no Secret or customer content.
    - [ ] The implementation follows `CONTEXT.md`, ADR 0024, and the Platform-owned business fact boundary.

    ## Blocked by

    #{blockers.empty? ? '- None — can start immediately' : blockers.map { |n| "- ##{n}" }.join("\n")}
  MARKDOWN

  args = ["gh", "issue", "create", "--repo", REPO, "--title", item.fetch(:title), "--body", body,
          "--label", "ready-for-agent", "--parent", "2"]
  blockers.each { |number| args.concat(["--blocked-by", number.to_s]) }
  url = run!(*args)
  number = url[/\d+\z/].to_i
  created[index] = number
  puts "created #{url}"
end
