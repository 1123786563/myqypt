# Third-Party Notices

This file records the provenance of third-party material that this repository's
authored code derives from, as required by the whitelist-extraction design in
`docs/superpowers/specs/2026-08-24-shadcn-admin-go-admin-extraction-design.md`.
It covers derivations only: runtime and development dependencies of `web/` are
declared in `web/package.json`, and each dependency distributes its own license
text with its package.

## shadcn-admin — engineering configuration patterns and theming approach

- Upstream project: <https://github.com/satnaing/shadcn-admin> (`satnaing/shadcn-admin`)
- Reviewed revision: commit `e16c87f213a5ba5e45964e9b67c792105ec74d26`
- License: MIT (license text in the upstream repository's `LICENSE` file)

The React landing site under `web/` was produced under a whitelist-extraction
process: engineering ideas and structure were taken from the upstream review,
while the upstream's product surface was deliberately left behind. The reviewed
commit is a provenance record, not a sync target; any future upstream change must
be re-reviewed as a new diff, never merged over these files.

### Adopted from the upstream review

1. **Engineering-configuration arrangement** — how Vite, TypeScript, ESLint,
   Prettier, and a Vitest + Playwright test harness compose into one project. The
   corresponding files in `web/` (`vite.config.ts`, `tsconfig.json`,
   `eslint.config.js`, `.prettierrc.json`, `vitest.config.ts`,
   `playwright.config.ts`, and the `scripts/` checkers) were re-authored for this
   repository; no upstream configuration file was copied.
2. **Tailwind CSS 4 + shadcn CSS-variables theming approach** — re-implemented in
   `web/src/styles/app.css`: semantic CSS custom properties
   (`--background`, `--foreground`, `--primary`, …), `oklch()` color values, and a
   `.dark` color-scheme variant, following the theming approach documented by
   shadcn and consumed by the upstream project.
3. **Component conventions** — composition over configuration, `cva`-based variant
   APIs, `data-slot` markers, and `cn()` class merging, demonstrated concretely by
   the shadcn/ui Button copy documented in the next section.

### Deliberately NOT copied from the upstream

- **Clerk authentication** — no `@clerk/*` dependency, Clerk route, or Clerk
  component exists here. Platform identity is owned by the Go/Keycloak side of the
  platform and is out of scope for the static landing.
- **Demo domain models** — the upstream's Tasks, Chats, Users, and Apps demo CRUD
  features (data, routes, and UI) were not carried over. This project's routes are
  its own (`/` today; `/products`, `/pricing`, and `/app` are planned platform
  routes).
- **Brand assets and marketing copy** — no upstream logo, illustration, wordmark,
  or landing copy was copied; all copy in `web/src` is authored for myqypt.
- **Upstream lockfile** — `web/pnpm-lock.yaml` is generated from this repository's
  own `web/package.json`; the upstream lockfile was not reused.
- **Upstream route tree** — the upstream's generated router tree
  (`routeTree.gen.ts`) was not copied; this project uses React Router
  framework-mode route configuration (`web/src/routes.ts`).

### Local modifications

For the re-authored items above there is no copied code to modify. The one
substantially copied file is the Button component below, whose complete delta is
listed there. `web/scripts/check-forbidden-content.mjs` enforces the "not copied"
list mechanically: Clerk references, token-like storage keys, demo route segments,
the upstream brand string, React Router's internal asset namespace, and `localhost`
are rejected in authored sources (`pnpm --dir web verify:forbidden`).

## shadcn/ui Button component — canonical copy

- Source: the canonical `button.tsx` published by the shadcn/ui project
  (<https://github.com/shadcn-ui/ui>, Tailwind CSS v4 "new-york" style), MIT
  licensed and distributed via the shadcn CLI.
- Local copy: `web/src/components/ui/button.tsx`, locally maintained — upstream is
  not a dependency and is not fetched at build time.
- Complete list of local differences from the canonical file:
  1. The `cn` utility import path is `../../lib/utils` instead of the canonical
     `@/lib/utils` alias (this project configures no path alias).
  2. The JSDoc comment on the `asChild` prop is translated into Chinese.
  3. Formatting normalized by Prettier (single quotes, no semicolons).
  4. The `destructive` variant preset is omitted: the local variant set is
     `default` / `secondary` / `outline` / `ghost` / `link` only. As a
     consequence there is no destructive styling or API locally —
     `buttonVariants({ variant: 'destructive' })` is rejected by TypeScript —
     and reintroducing it later requires adding the destructive color tokens
     (e.g. `--destructive`, `--destructive-foreground`) to
     `web/src/styles/app.css`.
  5. The `aria-invalid:*` state classes (`aria-invalid:border-ring`,
     `aria-invalid:ring-destructive/20`, `aria-invalid:border`) are omitted
     from the base class string: the canonical file styles the invalid state
     through those variants, the local copy does not.
- Caveat: the token-level class details of the canonical file evolve across
  upstream revisions (e.g. border-token and shadow details), so this list
  tracks structural/functional deltas against the canonical Tailwind v4
  "new-york" button as published at extraction time (2026-08), not
  line-by-line class parity.
- Functional status: `asChild`, the size presets, and the variant presets
  that are present behave as in the canonical file, while the two omissions
  above are deliberate functional reductions made for this extraction — no
  consumer needs `destructive` or `aria-invalid` styling today.

## MIT License

Both upstream projects referenced above are licensed under the MIT License:

> Permission is hereby granted, free of charge, to any person obtaining a copy of
> this software and associated documentation files (the "Software"), to deal in
> the Software without restriction, including without limitation the rights to
> use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies of
> the Software, and to permit persons to whom the Software is furnished to do so,
> subject to the following conditions:
>
> The above copyright notice and this permission notice shall be included in all
> copies or substantial portions of the Software.
>
> THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
> IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY, FITNESS
> FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR
> COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER
> IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN
> CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.

The copyright notice line that each upstream's license requires to be preserved
with the material derived from it (as quoted from that upstream's `LICENSE` file
at the revision recorded above):

- `satnaing/shadcn-admin` — engineering configuration patterns and theming
  approach: `Copyright (c) 2024 Sat Naing`
- `shadcn/ui` — the Button component copy at `web/src/components/ui/button.tsx`:
  `Copyright (c) 2023 shadcn`
