// Frontend dependency policy gate for Issue #105 (F05), design ruling 6.
//
// Dependency level: parse web/package.json (every dependency section) and
// web/pnpm-lock.yaml (package names). Rule FRONTEND-DEP-CLERK: a dependency
// package name matching /clerk/i — Clerk auth must never enter the project.
//
// Source level: delegate read-only to the existing F06 gate
// web/scripts/check-forbidden-content.mjs, passing the scan root. Its failure
// is classified FRONTEND-SOURCE-CONTENT here.
//
// Usage:
//   node scripts/check-frontend-policy.mjs                 # scan the real web/
//   node scripts/check-frontend-policy.mjs --fixture <dir> # scan a fixture tree
//
// The fixture mode exists so the gate can be exercised against crafted trees
// without touching real sources. A missing package.json or pnpm-lock.yaml is
// a failure (refusing vacuous passes), as is a scan root with no files.
import { spawnSync } from 'node:child_process'
import { existsSync, readFileSync } from 'node:fs'
import path from 'node:path'
import process from 'node:process'
import { fileURLToPath } from 'node:url'

const repoRoot = path.resolve(fileURLToPath(new URL('..', import.meta.url)))
const realWebRoot = path.join(repoRoot, 'web')
const sourceChecker = path.join(repoRoot, 'web', 'scripts', 'check-forbidden-content.mjs')

const DEP_SECTIONS = ['dependencies', 'devDependencies', 'peerDependencies', 'optionalDependencies']
const DEP_RULE = { id: 'FRONTEND-DEP-CLERK', pattern: /clerk/i }
const SOURCE_RULE_ID = 'FRONTEND-SOURCE-CONTENT'

function parseArgs(argv) {
  const args = { fixture: null }
  for (let i = 0; i < argv.length; i += 1) {
    if (argv[i] === '--fixture' && argv[i + 1]) {
      args.fixture = argv[i + 1]
      i += 1
    } else {
      console.error(`check-frontend-policy: unknown argument: ${argv[i]}`)
      console.error('usage: node scripts/check-frontend-policy.mjs [--fixture <dir>]')
      process.exit(2)
    }
  }
  return args
}

// Collects package names from every dependency section of package.json.
function packageJsonNames(webRoot) {
  const file = path.join(webRoot, 'package.json')
  if (!existsSync(file)) {
    return { error: `package.json not found under ${webRoot}; refusing vacuous pass` }
  }
  const pkg = JSON.parse(readFileSync(file, 'utf8'))
  const found = []
  for (const section of DEP_SECTIONS) {
    const deps = pkg[section]
    if (!deps || typeof deps !== 'object') continue
    for (const name of Object.keys(deps)) found.push({ where: `package.json ${section}`, name })
  }
  return { found }
}

// Extracts a package name from a pnpm-lock.yaml entry like
// "@scope/name@1.2.3" or "react@19.2.8" (quotes already stripped).
function packageNameFromEntry(entry) {
  if (entry.startsWith('@')) return `@${entry.slice(1).split('@')[0]}`
  return entry.split('@')[0]
}

// Collects package names from pnpm-lock.yaml: importer dependency keys
// (indented `name:` lines) and package/snapshot entries (`name@version:`).
function lockfileNames(webRoot) {
  const file = path.join(webRoot, 'pnpm-lock.yaml')
  if (!existsSync(file)) {
    return { error: `pnpm-lock.yaml not found under ${webRoot}; refusing vacuous pass` }
  }
  const found = []
  const lines = readFileSync(file, 'utf8').split('\n')
  for (const line of lines) {
    // name@version entries (version starts with a digit); checked first so
    // their inner @ never gets swallowed by the dependency-key pattern.
    const entry = line.match(/^\s+'?([@.\w/-]+@[\d][^:\s]*)'?:$/)
    if (entry) {
      found.push({ where: 'pnpm-lock.yaml packages', name: packageNameFromEntry(entry[1]) })
      continue
    }
    // importer dependency keys: a bare (possibly scoped) name with no version
    const depKey = line.match(/^\s+'?(@[\w.-]+\/[\w.-]+|[\w.-]+)'?:$/)
    if (depKey && !/^(?:lockfileVersion|importers|packages|snapshots|dependencies|devDependencies|peerDependencies|optionalDependencies)$/.test(depKey[1])) {
      found.push({ where: 'pnpm-lock.yaml importers', name: depKey[1] })
    }
  }
  return { found }
}

// Runs the F06 source-content gate against webRoot and classifies failures.
function runSourceCheck(webRoot) {
  const result = spawnSync(process.execPath, [sourceChecker, webRoot], { stdio: 'inherit' })
  if (result.error) {
    return { id: SOURCE_RULE_ID, detail: `cannot run ${sourceChecker}: ${result.error.message}` }
  }
  if (result.status !== 0) {
    return { id: SOURCE_RULE_ID, detail: `source scan exited ${result.status} (see output above)` }
  }
  return null
}

function main() {
  const { fixture } = parseArgs(process.argv.slice(2))
  const webRoot = fixture ? path.resolve(process.cwd(), fixture) : realWebRoot

  const violations = []
  const depSources = [packageJsonNames(webRoot), lockfileNames(webRoot)]
  let depCount = 0
  for (const source of depSources) {
    if (source.error) {
      console.error(`check-frontend-policy: FAIL ${source.error}`)
      process.exit(1)
    }
    for (const dep of source.found) {
      depCount += 1
      if (DEP_RULE.pattern.test(dep.name)) {
        violations.push({
          id: DEP_RULE.id,
          detail: `dependency "${dep.name}" in ${dep.where} matches forbidden pattern ${DEP_RULE.pattern}`,
        })
      }
    }
  }
  if (depCount === 0) {
    console.error(`check-frontend-policy: FAIL no dependency entries found under ${webRoot}; refusing vacuous pass`)
    process.exit(1)
  }

  const sourceViolation = runSourceCheck(webRoot)
  if (sourceViolation) violations.push(sourceViolation)

  if (violations.length > 0) {
    for (const v of violations) console.error(`FAIL: [${v.id}] ${v.detail}`)
    console.error(`check-frontend-policy: FAILED with ${violations.length} violation(s) (root: ${webRoot})`)
    process.exit(1)
  }
  console.log(`check-frontend-policy: OK (${depCount} dependency entries checked, source scan passed) -> ${webRoot}`)
}

main()
