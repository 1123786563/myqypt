// Forbid upstream residue and credential-shaped content in the web project's authored
// sources.
//
// Why this gate exists (issue #106, design spec docs/superpowers/specs/
// 2026-08-24-shadcn-admin-go-admin-extraction-design.md §5.4): the web landing is
// produced by whitelist extraction from satnaing/shadcn-admin (MIT). Clerk auth,
// token-like storage keys, the upstream demo domain (tasks/chats/users/apps),
// upstream brand strings, React Router's internal asset namespace, and non-portable
// host references must never enter the authored tree.
//
// Scan scope, relative to the web project root (this script lives in web/scripts/):
//   - src/, tests/, scripts/ recursively (text files only)
//   - root config files: package.json and *.config.{ts,js,json} directly under web/
// Deliberately never scanned: build/, node_modules/, .react-router/,
// pnpm-lock.yaml, playwright-report/, test-results/ — generated output and
// lockfiles are not authored content.
//
// Usage: node scripts/check-forbidden-content.mjs [web-root]
//   web-root  optional alternative scan root, resolved against the current working
//             directory; defaults to the web project shipping this script. The
//             override exists so the gate can be red-tested against a crafted
//             fixture tree without touching real sources.
//
// Output: one `FAIL:` line per violation (file:line:col, rule, matched text) and
// exit code 1; or a one-line OK summary and exit code 0.
import { existsSync, readdirSync, readFileSync } from 'node:fs'
import path from 'node:path'
import process from 'node:process'
import { fileURLToPath } from 'node:url'

const defaultWebRoot = path.resolve(fileURLToPath(new URL('..', import.meta.url)))
const cliRoot = process.argv[2]
const webRoot = cliRoot ? path.resolve(process.cwd(), cliRoot) : defaultWebRoot

// Directories never descended into while walking src/, tests/, scripts/.
const SKIP_DIRS = new Set(['build', 'dist', 'node_modules', 'playwright-report', 'test-results'])

// Only text files can be meaningfully scanned; anything else (binaries, dotfiles
// such as .DS_Store, editor droppings) is skipped and left to review.
const TEXT_EXTENSIONS = new Set([
  '.css',
  '.html',
  '.js',
  '.cjs',
  '.jsx',
  '.json',
  '.mjs',
  '.md',
  '.svg',
  '.ts',
  '.tsx',
  '.txt',
])

// Root config files scanned in addition to the walked directories (exact scope
// mandated by the plan: package.json plus *.config.{ts,js,json} at the web root).
const ROOT_CONFIG_FILE = /^(?:package\.json|[\w.-]+\.config\.(?:ts|js|json))$/

// Files exempt from the forbidden-pattern rules below.
//
// Guard scripts necessarily contain the forbidden strings as literal detection
// patterns — that is the data they match against, not shipped content. The
// exemption is an explicit, path-scoped list (chosen over hiding the literals via
// concatenated string fragments) so it stays visible in review. Both entries are
// themselves gates whose own runs and reviews police their content.
const PATTERN_LITERAL_EXEMPT_FILES = new Set([
  'scripts/check-forbidden-content.mjs', // this scanner: holds every pattern as data
  'scripts/assert-static-build.mjs', // build checker: clerk/localhost/_react-router_ literals
])

// Storage keys explicitly allowed to be read or written via localStorage/sessionStorage.
// Every entry is a deliberate, reviewed exception to the token-like-key rules;
// each new entry must carry an equivalent justification comment.
const ALLOWED_STORAGE_KEYS = new Set([
  // 'ui-theme' persists only the visitor's light/dark theme preference (values are
  // exactly 'light' | 'dark'). It is not a credential, session, or tracking
  // artifact, so a static marketing site may keep it in localStorage (used by
  // web/src/root.tsx and web/src/components/theme-toggle.tsx).
  'ui-theme',
  // 'myqypt.appearance.v1' persists only the console appearance preferences as a
  // versioned three-field JSON object ({ theme: 'light'|'dark'|'system',
  // density: 'comfortable'|'compact', sidebarCollapsed: boolean }). It is not a
  // credential, session, or tracking artifact (used by
  // web/src/components/platform/appearance-preferences.tsx, its appearance-store
  // module, and the pre-paint script in web/src/root.tsx).
  'myqypt.appearance.v1',
])

// One regex per forbidden literal or marker, checked line by line so each violation
// can be reported with the file:line:col of the offending match.
const LINE_RULES = [
  { id: 'clerk', label: 'Clerk auth reference (@clerk or clerk route)', pattern: /clerk/i },
  { id: 'bearer', label: 'literal "Bearer" (Authorization header scheme)', pattern: /\bBearer\b/ },
  { id: 'brand', label: 'upstream brand string (shadcn-admin)', pattern: /shadcn-admin/i },
  {
    id: 'demo-route',
    label: 'upstream demo route segment (tasks/chats/users/apps)',
    pattern: /['"`/](?:tasks|chats|users|apps)\b/,
  },
  {
    id: 'rr-internal',
    label: 'React Router internal asset namespace (/_react-router/)',
    pattern: /\/_react-router\//,
  },
  { id: 'localhost', label: 'non-portable host reference (localhost)', pattern: /localhost/i },
  {
    id: 'token-key',
    label: 'token-like string key (access-token / idToken / refresh token etc.)',
    pattern: /['"`][^'"`\n]*token[^'"`\n]*['"`]/i,
  },
]

// Composite rule: any localStorage/sessionStorage usage whose nearby key or property
// name matches the sensitive-word regex. "Nearby" is the storage line plus one line
// above and two below, so wrapped calls and just-declared key constants are covered.
const STORAGE_API = /\b(?:local|session)Storage\b/
const SENSITIVE_KEY_WORDS = /token|credential|secret|auth/gi
const STRING_OR_IDENTIFIER = /['"`][^'"`\n]*['"`]|[A-Za-z_$][\w$]*/g

function collectFiles() {
  const files = []
  const walk = (dir) => {
    let entries
    try {
      entries = readdirSync(dir, { withFileTypes: true })
    } catch {
      return // directory absent: not every fixture tree has src/tests/scripts
    }
    entries.sort((a, b) => a.name.localeCompare(b.name))
    for (const entry of entries) {
      const child = path.join(dir, entry.name)
      if (entry.isDirectory()) {
        if (entry.name.startsWith('.') || SKIP_DIRS.has(entry.name)) continue
        walk(child)
      } else if (
        entry.isFile() &&
        !entry.name.startsWith('.') &&
        TEXT_EXTENSIONS.has(path.extname(entry.name).toLowerCase())
      ) {
        files.push(child)
      }
    }
  }
  for (const dir of ['src', 'tests', 'scripts']) {
    walk(path.join(webRoot, dir))
  }
  let rootEntries = []
  try {
    rootEntries = readdirSync(webRoot, { withFileTypes: true })
  } catch {
    rootEntries = [] // surfaced by the webRoot existence check in main()
  }
  for (const entry of rootEntries) {
    if (entry.isFile() && ROOT_CONFIG_FILE.test(entry.name)) {
      files.push(path.join(webRoot, entry.name))
    }
  }
  return files
}

function checkLineRules(relPath, lines, violations) {
  lines.forEach((line, index) => {
    for (const rule of LINE_RULES) {
      const flags = rule.pattern.flags.includes('g') ? rule.pattern.flags : `${rule.pattern.flags}g`
      for (const match of line.matchAll(new RegExp(rule.pattern.source, flags))) {
        violations.push({
          relPath,
          line: index + 1,
          column: match.index + 1,
          id: rule.id,
          label: rule.label,
          matched: match[0],
        })
      }
    }
  })
}

// Returns the quoted string or identifier token of `text` that contains `index`.
function containingToken(text, index) {
  for (const match of text.matchAll(STRING_OR_IDENTIFIER)) {
    if (index >= match.index && index < match.index + match[0].length) return match[0]
  }
  return text.slice(index, index + 16)
}

function checkStorageKeys(relPath, lines, violations) {
  for (let i = 0; i < lines.length; i += 1) {
    if (!STORAGE_API.test(lines[i])) continue
    const windowText = lines.slice(Math.max(0, i - 1), Math.min(lines.length, i + 3)).join('\n')
    for (const match of windowText.matchAll(SENSITIVE_KEY_WORDS)) {
      const token = containingToken(windowText, match.index)
      const key = token.replace(/^['"`]|['"`]$/g, '').toLowerCase()
      if (ALLOWED_STORAGE_KEYS.has(key)) continue
      const before = windowText.slice(0, match.index)
      const line = Math.max(0, i - 1) + (before.match(/\n/g) ?? []).length
      const column = match.index - (before.lastIndexOf('\n') + 1) + 1
      violations.push({
        relPath,
        line: line + 1,
        column,
        id: 'sensitive-storage-key',
        label: `token-like key near localStorage/sessionStorage (allowed storage keys: ${[
          ...ALLOWED_STORAGE_KEYS,
        ].join(', ')})`,
        matched: token,
      })
    }
  }
}

function main() {
  if (!existsSync(webRoot)) {
    console.error(`check-forbidden-content: scan root does not exist: ${webRoot}`)
    process.exit(1)
  }
  const files = collectFiles()
  if (files.length === 0) {
    console.error(
      `check-forbidden-content: no scannable files under ${webRoot}, refusing vacuous OK`,
    )
    process.exit(1)
  }
  const violations = []
  let exemptCount = 0
  for (const file of files) {
    const relPath = path.relative(webRoot, file).split(path.sep).join('/')
    if (PATTERN_LITERAL_EXEMPT_FILES.has(relPath)) {
      exemptCount += 1
      continue
    }
    const lines = readFileSync(file, 'utf8').split('\n')
    checkLineRules(relPath, lines, violations)
    checkStorageKeys(relPath, lines, violations)
  }
  violations.sort((a, b) =>
    a.relPath === b.relPath
      ? a.line - b.line || a.column - b.column
      : a.relPath < b.relPath
        ? -1
        : 1,
  )
  if (violations.length > 0) {
    for (const v of violations) {
      console.error(
        `FAIL: ${v.relPath}:${v.line}:${v.column} [${v.id}] ${v.label} | matched: ${JSON.stringify(v.matched)}`,
      )
    }
    console.error(
      `check-forbidden-content: FAILED with ${violations.length} violation(s) (root: ${webRoot})`,
    )
    process.exit(1)
  }
  console.log(
    `check-forbidden-content: OK (${files.length} file(s) scanned, ` +
      `${exemptCount} guard-file exemption(s) applied) -> ${webRoot}`,
  )
}

main()
