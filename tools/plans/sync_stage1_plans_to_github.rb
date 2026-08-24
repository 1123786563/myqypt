#!/usr/bin/env ruby
# frozen_string_literal: true

require "json"
require "open3"
require "thread"

REPO = "1123786563/myqypt"
ROOT = File.expand_path("../..", __dir__)
PLAN_DIR = File.join(ROOT, "docs", "superpowers", "plans")
MARKER = "## Implementation plan"

def run!(*args)
  last_error = nil
  8.times do |attempt|
    out, err, status = Open3.capture3(*args)
    return out.strip if status.success?
    last_error = err
    sleep([2**attempt, 16].min)
  end
  raise "#{args.first(5).join(' ')} failed after retries: #{last_error}"
end

plans = {}
Dir[File.join(PLAN_DIR, "2026-08-24-*.md")].sort.each do |path|
  next if path.end_with?("stage-1-index.md")
  content = File.read(path)
  number = content[/GitHub Issue #(\d+)/, 1]
  raise "missing Issue number in #{path}" unless number
  number = number.to_i
  raise "duplicate plan for Issue ##{number}" if plans.key?(number)
  plans[number] = [path, content]
end

raise "expected 100 plans, found #{plans.length}" unless plans.length == 100
raise "plan issue range mismatch" unless plans.keys.sort == (2..101).to_a

raw = run!("gh", "api", "--paginate", "repos/#{REPO}/issues?state=all&per_page=100")
issues = JSON.parse(raw).reject { |issue| issue.key?("pull_request") }.to_h { |issue| [issue.fetch("number"), issue] }

jobs = plans.keys.sort.each_with_index.map do |number, index|
  path, plan = plans.fetch(number)
  issue = issues.fetch(number)
  base = issue.fetch("body").sub(/\n#{Regexp.escape(MARKER)}\n.*\z/m, "").rstrip
  relative = path.delete_prefix("#{ROOT}/")
  body = <<~MARKDOWN.rstrip
    #{base}

    #{MARKER}

    Source: `#{relative}`

    #{plan}
  MARKDOWN
  raise "Issue ##{number} body exceeds GitHub limit" if body.bytesize > 65_536
  issue.fetch("body") == body ? nil : [index, number, body]
end
jobs.compact!

STDOUT.sync = true
queue = Queue.new
jobs.each { |job| queue << job }
errors = Queue.new
completed = plans.length - jobs.length
mutex = Mutex.new
puts "already current #{completed}/#{plans.length}; pending #{jobs.length}"

workers = 2.times.map do
  Thread.new do
    loop do
      index, number, body = queue.pop(true)
      run!("gh", "api", "--method", "PATCH", "repos/#{REPO}/issues/#{number}", "-f", "body=#{body}")
      mutex.synchronize do
        completed += 1
        puts "synced #{completed}/#{plans.length}: Issue ##{number} (#{body.bytesize} bytes)" if (completed % 5).zero? || completed == 1 || completed == plans.length
      end
    rescue ThreadError
      break
    rescue StandardError => e
      errors << [number, e]
      break
    end
  end
end
workers.each(&:join)

unless errors.empty?
  number, error = errors.pop
  raise "sync failed at Issue ##{number}: #{error.message}"
end
raise "only synced #{completed}/#{plans.length}" unless completed == plans.length
