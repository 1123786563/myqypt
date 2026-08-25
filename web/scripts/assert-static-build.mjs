// Assert the built landing page is a CDN-ready, JavaScript-independent static site.
//
// Usage: node scripts/assert-static-build.mjs [target-dir]
//   target-dir  optional path to the static output directory, resolved against the
//               current working directory (works from web/ and from the repo root);
//               defaults to <web>/build/client resolved against this script so
//               `pnpm --dir web verify:static` works from anywhere.
//
// Exits non-zero when any assertion below fails (one `FAIL:` line per violation):
//   - index.html exists
//   - non-empty <title>, meta description, rel="canonical" with a non-empty href,
//     and visible <h1> text
//   - at least one content-hashed /assets/*.css|.js reference, and every
//     site-relative referenced asset exists on disk under the target directory
//   - no /_react-router/, clerk (case-insensitive), localhost, or server-runtime
//     imports (@react-router/node, @react-router/serve, "server-only",
//     entry.server, /build/server)
import { existsSync, readFileSync, readdirSync } from 'node:fs'
import path from 'node:path'
import process from 'node:process'
import { fileURLToPath } from 'node:url'

const projectRoot = path.resolve(fileURLToPath(new URL('..', import.meta.url)))
const cliTarget = process.argv[2]
const targetDir = cliTarget
  ? path.resolve(process.cwd(), cliTarget)
  : path.join(projectRoot, 'build', 'client')

const errors = []
const fail = (message) => {
  errors.push(message)
}

const indexPath = path.join(targetDir, 'index.html')

if (!existsSync(indexPath)) {
  fail(`missing ${indexPath}`)
} else {
  const html = readFileSync(indexPath, 'utf8')

  const title = html.match(/<title[^>]*>([\s\S]*?)<\/title>/i)
  if (!title || title[1].trim() === '') {
    fail('index.html must contain a non-empty <title>')
  }

  const hasDescription = [...html.matchAll(/<meta\b[^>]*>/gi)].some((tag) => {
    const name = /\bname\s*=\s*["']description["']/i.test(tag[0])
    const content = tag[0].match(/\bcontent\s*=\s*["']([^"']*)["']/i)
    return name && Boolean(content) && content[1].trim() !== ''
  })
  if (!hasDescription) {
    fail('index.html must contain <meta name="description"> with non-empty content')
  }

  const hasCanonical = [...html.matchAll(/<link\b[^>]*>/gi)].some((tag) => {
    const rel = /\brel\s*=\s*["']canonical["']/i.test(tag[0])
    const href = tag[0].match(/\bhref\s*=\s*["']([^"']*)["']/i)
    return rel && Boolean(href) && href[1].trim() !== ''
  })
  if (!hasCanonical) {
    fail('index.html must contain <link rel="canonical"> with a non-empty href')
  }

  const decodeText = (value) =>
    value
      .replace(/<[^>]*>/g, ' ')
      .replace(/&nbsp;/gi, ' ')
      .replace(/&amp;/gi, '&')
      .replace(/&lt;/gi, '<')
      .replace(/&gt;/gi, '>')
      .replace(/&quot;/gi, '"')
      .replace(/&#x27;|&#39;/gi, "'")
  const hasVisibleHeading = [...html.matchAll(/<h1\b[^>]*>([\s\S]*?)<\/h1>/gi)].some(
    (match) => decodeText(match[1]).trim() !== '',
  )
  if (!hasVisibleHeading) {
    fail('index.html must contain an <h1> with visible (non-whitespace) text')
  }

  const assetRefs = [...html.matchAll(/(?:href|src)\s*=\s*["']([^"']*\/assets\/[^"']+)["']/gi)].map(
    (match) => match[1],
  )
  const hasHashedAsset = assetRefs.some((ref) => /[0-9a-zA-Z]{8,}\.(?:css|js)(?:[?#]|$)/.test(ref))
  if (!hasHashedAsset) {
    fail(
      'index.html must reference at least one content-hashed asset (an /assets/* .css or ' +
        '.js URL with an 8+ hex/alphanumeric hash segment before the extension)',
    )
  }
  for (const ref of [...new Set(assetRefs)]) {
    if (/^[a-z][a-z0-9+.-]*:/i.test(ref)) {
      continue // absolute URL with a scheme: cannot be checked against the local target
    }
    let relativePath
    try {
      relativePath = decodeURIComponent(ref.split('?')[0].split('#')[0]).replace(/^\/+/, '')
    } catch {
      fail(`referenced asset URL is malformed: ${ref}`)
      continue
    }
    if (!existsSync(path.join(targetDir, relativePath))) {
      fail(`referenced asset missing on disk: ${ref} (expected under ${targetDir})`)
    }
  }
}

const forbiddenPatterns = [
  { pattern: /\/_react-router\//, label: '"/_react-router/" (SSR runtime reference)' },
  { pattern: /clerk/i, label: '"clerk" (auth provider reference)' },
  { pattern: /localhost/, label: '"localhost" (non-portable host reference)' },
  {
    pattern: /@react-router\/(?:node|serve)|(["'])server-only\1/,
    label: 'server-runtime import (@react-router/node, @react-router/serve, "server-only")',
  },
  {
    pattern: /\/build\/server|entry\.server/,
    label: 'server build reference (entry.server / /build/server)',
  },
]

const htmlFiles = existsSync(targetDir)
  ? readdirSync(targetDir)
      .filter((name) => name.endsWith('.html'))
      .sort()
  : []
for (const name of htmlFiles) {
  const content = readFileSync(path.join(targetDir, name), 'utf8')
  for (const { pattern, label } of forbiddenPatterns) {
    if (pattern.test(content)) {
      fail(`${name} must not contain ${label}`)
    }
  }
}

if (errors.length > 0) {
  for (const error of errors) {
    console.error(`FAIL: ${error}`)
  }
  console.error(
    `assert-static-build: FAILED with ${errors.length} problem${errors.length === 1 ? '' : 's'} (target: ${targetDir})`,
  )
  process.exit(1)
}

console.log(`assert-static-build: OK (${htmlFiles.length} HTML file(s)) -> ${targetDir}`)
