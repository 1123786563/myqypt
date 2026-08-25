#!/usr/bin/env ruby
# frozen_string_literal: true

# Sync implementation plans to GitHub issue bodies.
#
# Contract (reworked 2026-08-25, see tools/plans/README.md):
# - Scans docs/superpowers/plans/2026-*.md for any series (T/F/U/...).
# - A plan belongs to the issue whose number appears in its "**Spec:**" line
#   as "Issue #N"; files without a Spec line (indexes, templates, stubs) are
#   skipped explicitly, never guessed.
# - The issue body keeps everything before the "## Implementation plan"
#   marker; the marker section is replaced with "Source: <relative path>"
#   plus the plan file content. Bodies that already match are left alone.
# - Usage: ruby sync_stage1_plans_to_github.rb [--dry-run]

require "json"
require "open3"

REPO = "1123786563/myqypt"
ROOT = File.expand_path("../..", __dir__)
PLAN_DIR = File.join(ROOT, "docs", "superpowers", "plans")
MARKER = "## Implementation plan"
DRY_RUN = ARGV.delete("--dry-run")
SKIP_PATTERNS = [/index\.md\z/, /template\.md\z/].freeze

def run!(*args)
  last_error = nil
  8.times do |attempt|
    out, err, status = Open3.capture3(*args)
    return out.strip if status.success?
    last_error = err
    sleep([2**attempt, 16].min)
  end
  raise "#{args.first(5).join(" ")} failed after retries: #{last_error}"
end

plans = {}
skipped = []
Dir[File.join(PLAN_DIR, "2026-*.md")].sort.each do |path|
  next if SKIP_PATTERNS.any? { |pattern| path.match?(pattern) }

  content = File.read(path)
  spec_line = content.lines.find { |line| line.start_with?("**Spec:**") }
  number = spec_line&.[](/Issue #(\d+)/, 1)
  if number.nil?
    skipped << File.basename(path)
    next
  end
  number = number.to_i
  if plans.key?(number)
    raise "duplicate plan for Issue ##{number}: #{File.basename(plans.fetch(number).first)} and #{File.basename(path)}"
  end
  plans[number] = [path, content]
end

puts "collected #{plans.length} plans (issues #{plans.keys.min}..#{plans.keys.max}); skipped #{skipped.length} file(s)#{skipped.empty? ? "" : ": #{skipped.join(", ")}"}"

raw = run!("gh", "api", "--paginate", "repos/#{REPO}/issues?state=all&per_page=100")
issues = JSON.parse(raw).reject { |issue| issue.key?("pull_request") }.to_h { |issue| [issue.fetch("number"), issue] }
missing = plans.keys.sort - issues.keys
raise "no GitHub issue for ##{missing.join(", #")}" unless missing.empty?

jobs = plans.keys.sort.map do |number|
  path, plan = plans.fetch(number)
  issue = issues.fetch(number)
  base = issue.fetch("body").to_s.sub(/\n#{Regexp.escape(MARKER)}\n.*\z/m, "").rstrip
  relative = path.delete_prefix("#{ROOT}/")
  body = <<~MARKDOWN.rstrip
    #{base}

    #{MARKER}

    Source: #{relative}

    #{plan}
  MARKDOWN
  raise "Issue ##{number} body exceeds GitHub limit" if body.bytesize > 65_536

  issue.fetch("body").to_s == body ? nil : [number, body, relative]
end.compact

puts jobs.empty? ? "already current" : "pending #{jobs.length}/#{plans.length}"

if DRY_RUN
  jobs.each { |number, _body, relative| puts "would sync Issue ##{number} (#{relative})" }
  exit 0
end

queue = Queue.new
jobs.each { |job| queue << job }
errors = Queue.new
completed = plans.length - jobs.length
mutex = Mutex.new
STDOUT.sync = true

workers = 2.times.map do
  Thread.new do
    loop do
      number, body, relative = queue.pop(true)
      run!("gh", "api", "--method", "PATCH", "repos/#{REPO}/issues/#{number}", "-f", "body=#{body}")
      mutex.synchronize do
        completed += 1
        puts "synced #{completed}/#{plans.length}: Issue ##{number} (#{relative})"
      end
    rescue ThreadError
      break
    rescue StandardError => e
      errors << [number, e]
    end
  end
end
workers.each(&:join)

unless errors.empty?
  errors.each { |number, e| warn "Issue ##{number}: #{e}" }
  raise "#{errors.size} sync failure(s)"
end
