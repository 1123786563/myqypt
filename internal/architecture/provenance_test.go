package architecture

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// validMITText is a minimal MIT license text satisfying PROV-LICENSE-TEXT.
const validMITText = `MIT License

Copyright (c) 2026 Example Corp

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction.
`

// writeFile creates rel (with parent directories) under dir.
func writeFile(t *testing.T, dir, rel, content string) {
	t.Helper()
	abs := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", abs, err)
	}
	if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", abs, err)
	}
}

func violationRuleIDs(vs []Violation) []string {
	ids := make([]string, 0, len(vs))
	for _, v := range vs {
		ids = append(ids, v.RuleID)
	}
	sort.Strings(ids)
	return ids
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// validPatternOnlyYAML is a fully valid pattern-only manifest (one source).
const validPatternOnlyYAML = `schema_version: 1
sources:
  - name: shadcn-admin
    repository: https://github.com/satnaing/shadcn-admin
    commit: e16c87f213a5ba5e45964e9b67c792105ec74d26
    license: MIT
    license_file: LICENSES/shadcn-admin-MIT.txt
    extraction_mode: pattern-only
    destinations: []
    local_modifications:
      - Re-authored engineering-configuration organization; zero verbatim copy.
`

func TestProvenanceRules(t *testing.T) {
	tests := []struct {
		name string
		// manifest is the in-memory manifest content under test.
		manifest string
		// setup, when non-nil, prepares a fresh temp repo root and returns it.
		setup func(t *testing.T) string
		// wantRules lists exactly the rule IDs that must fire.
		wantRules []string
		// wantInMessage, when set, must appear in every violation message.
		wantInMessage string
	}{
		{
			name:          "valid pattern-only manifest produces no violations",
			manifest:      validPatternOnlyYAML,
			setup:         func(t *testing.T) string { return licenseRoot(t, "LICENSES/shadcn-admin-MIT.txt") },
			wantRules:     nil,
			wantInMessage: "",
		},
		{
			name: "valid verbatim manifest with existing destination produces no violations",
			manifest: `schema_version: 1
sources:
  - name: widget
    repository: https://example.com/acme/widget
    commit: 0000000000000000000000000000000000000000
    license: MIT
    license_file: LICENSES/widget-MIT.txt
    extraction_mode: verbatim
    destinations:
      - web/src/copied.ts
    local_modifications: []
`,
			setup: func(t *testing.T) string {
				root := licenseRoot(t, "LICENSES/widget-MIT.txt")
				writeFile(t, root, "web/src/copied.ts", "copied\n")
				return root
			},
			wantRules: nil,
		},
		{
			name:      "schema_version not 1",
			manifest:  strings.Replace(validPatternOnlyYAML, "schema_version: 1", "schema_version: 2", 1),
			setup:     func(t *testing.T) string { return licenseRoot(t, "LICENSES/shadcn-admin-MIT.txt") },
			wantRules: []string{"PROV-SCHEMA-VERSION"},
		},
		{
			name:          "unparseable YAML",
			manifest:      "\t: : [not yaml",
			setup:         func(t *testing.T) string { return licenseRoot(t, "LICENSES/shadcn-admin-MIT.txt") },
			wantRules:     []string{"PROV-SCHEMA-VERSION"},
			wantInMessage: "schema_version",
		},
		{
			name: "empty sources",
			manifest: `schema_version: 1
sources: []
`,
			wantRules: []string{"PROV-SOURCE-EMPTY"},
		},
		{
			name:          "empty source name",
			manifest:      strings.Replace(validPatternOnlyYAML, "name: shadcn-admin", `name: ""`, 1),
			setup:         func(t *testing.T) string { return licenseRoot(t, "LICENSES/shadcn-admin-MIT.txt") },
			wantRules:     []string{"PROV-NAME"},
			wantInMessage: "source[0]",
		},
		{
			name: "duplicate source names",
			manifest: `schema_version: 1
sources:
  - name: shadcn-admin
    repository: https://github.com/satnaing/shadcn-admin
    commit: e16c87f213a5ba5e45964e9b67c792105ec74d26
    license: MIT
    license_file: LICENSES/a-MIT.txt
    extraction_mode: pattern-only
    destinations: []
    local_modifications:
      - Re-authored theming approach; zero verbatim copy.
  - name: shadcn-admin
    repository: https://github.com/satnaing/shadcn-admin
    commit: e16c87f213a5ba5e45964e9b67c792105ec74d26
    license: MIT
    license_file: LICENSES/a-MIT.txt
    extraction_mode: pattern-only
    destinations: []
    local_modifications:
      - Re-authored component conventions; zero verbatim copy.
`,
			setup:         func(t *testing.T) string { return licenseRoot(t, "LICENSES/a-MIT.txt") },
			wantRules:     []string{"PROV-NAME"},
			wantInMessage: "shadcn-admin",
		},
		{
			name:          "repository is not an https URL",
			manifest:      strings.Replace(validPatternOnlyYAML, "https://github.com/satnaing/shadcn-admin", "git@github.com:satnaing/shadcn-admin.git", 1),
			setup:         func(t *testing.T) string { return licenseRoot(t, "LICENSES/shadcn-admin-MIT.txt") },
			wantRules:     []string{"PROV-REPOSITORY"},
			wantInMessage: "shadcn-admin",
		},
		{
			name:          "commit is not lowercase hex",
			manifest:      strings.Replace(validPatternOnlyYAML, "e16c87f213a5ba5e45964e9b67c792105ec74d26", "E16C87F213A5BA5E45964E9B67C792105EC74D26", 1),
			setup:         func(t *testing.T) string { return licenseRoot(t, "LICENSES/shadcn-admin-MIT.txt") },
			wantRules:     []string{"PROV-COMMIT"},
			wantInMessage: "shadcn-admin",
		},
		{
			name:          "commit is a short sha",
			manifest:      strings.Replace(validPatternOnlyYAML, "e16c87f213a5ba5e45964e9b67c792105ec74d26", "e16c87f", 1),
			setup:         func(t *testing.T) string { return licenseRoot(t, "LICENSES/shadcn-admin-MIT.txt") },
			wantRules:     []string{"PROV-COMMIT"},
			wantInMessage: "shadcn-admin",
		},
		{
			name:          "license is not SPDX MIT",
			manifest:      strings.Replace(validPatternOnlyYAML, "license: MIT", "license: Apache-2.0", 1),
			setup:         func(t *testing.T) string { return licenseRoot(t, "LICENSES/shadcn-admin-MIT.txt") },
			wantRules:     []string{"PROV-LICENSE"},
			wantInMessage: "shadcn-admin",
		},
		{
			name:          "license file does not exist",
			manifest:      validPatternOnlyYAML,
			setup:         func(t *testing.T) string { return t.TempDir() },
			wantRules:     []string{"PROV-LICENSE-FILE-MISSING"},
			wantInMessage: "LICENSES/shadcn-admin-MIT.txt",
		},
		{
			name:     "license file escapes repo with ..",
			manifest: strings.Replace(validPatternOnlyYAML, "license_file: LICENSES/shadcn-admin-MIT.txt", "license_file: ../stolen-mit.txt", 1),
			setup: func(t *testing.T) string {
				root := t.TempDir()
				writeFile(t, filepath.Dir(root), "stolen-mit.txt", validMITText)
				return root
			},
			wantRules:     []string{"PROV-LICENSE-FILE-MISSING"},
			wantInMessage: "stolen-mit.txt",
		},
		{
			name:      "license file is empty",
			manifest:  validPatternOnlyYAML,
			setup:     func(t *testing.T) string { return licenseRoot(t, "LICENSES/shadcn-admin-MIT.txt", "") },
			wantRules: []string{"PROV-LICENSE-FILE-EMPTY"},
		},
		{
			name:     "license text lacks both MIT License and Copyright",
			manifest: validPatternOnlyYAML,
			setup: func(t *testing.T) string {
				return licenseRoot(t, "LICENSES/shadcn-admin-MIT.txt", "some unrelated text\n")
			},
			wantRules: []string{"PROV-LICENSE-TEXT"},
		},
		{
			name:     "license text has MIT License but no Copyright",
			manifest: validPatternOnlyYAML,
			setup: func(t *testing.T) string {
				return licenseRoot(t, "LICENSES/shadcn-admin-MIT.txt", "MIT License\n\nPermission is hereby granted.\n")
			},
			wantRules: []string{"PROV-LICENSE-TEXT"},
		},
		{
			name:          "extraction mode is not a legal enum",
			manifest:      strings.Replace(validPatternOnlyYAML, "extraction_mode: pattern-only", "extraction_mode: copy", 1),
			setup:         func(t *testing.T) string { return licenseRoot(t, "LICENSES/shadcn-admin-MIT.txt") },
			wantRules:     []string{"PROV-MODE"},
			wantInMessage: "shadcn-admin",
		},
		{
			name: "pattern-only declares destinations",
			manifest: `schema_version: 1
sources:
  - name: shadcn-admin
    repository: https://github.com/satnaing/shadcn-admin
    commit: e16c87f213a5ba5e45964e9b67c792105ec74d26
    license: MIT
    license_file: LICENSES/shadcn-admin-MIT.txt
    extraction_mode: pattern-only
    destinations:
      - web/src/pattern.ts
    local_modifications:
      - Re-authored engineering ideas; zero verbatim copy.
`,
			setup:         func(t *testing.T) string { return licenseRoot(t, "LICENSES/shadcn-admin-MIT.txt") },
			wantRules:     []string{"PROV-DEST-FORBIDDEN-MODE"},
			wantInMessage: "shadcn-admin",
		},
		{
			name:          "pattern-only without local modifications",
			manifest:      strings.Replace(validPatternOnlyYAML, "    local_modifications:\n      - Re-authored engineering-configuration organization; zero verbatim copy.\n", "    local_modifications: []\n", 1),
			setup:         func(t *testing.T) string { return licenseRoot(t, "LICENSES/shadcn-admin-MIT.txt") },
			wantRules:     []string{"PROV-MODIFICATIONS-EMPTY"},
			wantInMessage: "shadcn-admin",
		},
		{
			name: "verbatim without destinations",
			manifest: `schema_version: 1
sources:
  - name: widget
    repository: https://example.com/acme/widget
    commit: 0000000000000000000000000000000000000000
    license: MIT
    license_file: LICENSES/widget-MIT.txt
    extraction_mode: verbatim
    destinations: []
`,
			setup:         func(t *testing.T) string { return licenseRoot(t, "LICENSES/widget-MIT.txt") },
			wantRules:     []string{"PROV-DEST-EMPTY"},
			wantInMessage: "widget",
		},
		{
			name: "verbatim destination does not exist in repo",
			manifest: `schema_version: 1
sources:
  - name: widget
    repository: https://example.com/acme/widget
    commit: 0000000000000000000000000000000000000000
    license: MIT
    license_file: LICENSES/widget-MIT.txt
    extraction_mode: verbatim
    destinations:
      - web/src/never-created.ts
`,
			setup:         func(t *testing.T) string { return licenseRoot(t, "LICENSES/widget-MIT.txt") },
			wantRules:     []string{"PROV-DEST-MISSING"},
			wantInMessage: "widget",
		},
		{
			name: "verbatim destination is an absolute path",
			manifest: `schema_version: 1
sources:
  - name: widget
    repository: https://example.com/acme/widget
    commit: 0000000000000000000000000000000000000000
    license: MIT
    license_file: LICENSES/widget-MIT.txt
    extraction_mode: verbatim
    destinations:
      - /etc/passwd
`,
			setup:         func(t *testing.T) string { return licenseRoot(t, "LICENSES/widget-MIT.txt") },
			wantRules:     []string{"PROV-DEST-OUTSIDE-REPO"},
			wantInMessage: "widget",
		},
		{
			name: "verbatim destination escapes repo with ..",
			manifest: `schema_version: 1
sources:
  - name: widget
    repository: https://example.com/acme/widget
    commit: 0000000000000000000000000000000000000000
    license: MIT
    license_file: LICENSES/widget-MIT.txt
    extraction_mode: verbatim
    destinations:
      - ../outside-repo.ts
`,
			setup:         func(t *testing.T) string { return licenseRoot(t, "LICENSES/widget-MIT.txt") },
			wantRules:     []string{"PROV-DEST-OUTSIDE-REPO"},
			wantInMessage: "widget",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := "."
			if tt.setup != nil {
				root = tt.setup(t)
			}
			got := ValidateProvenance(root, []byte(tt.manifest))
			gotRules := violationRuleIDs(got)
			if !equalStrings(gotRules, tt.wantRules) {
				t.Errorf("ValidateProvenance rules = %v, want %v\nviolations:\n%s",
					gotRules, tt.wantRules, formatViolations(got))
			}
			if tt.wantInMessage != "" {
				for _, v := range got {
					if !strings.Contains(v.Message, tt.wantInMessage) {
						t.Errorf("violation %s message %q does not contain %q", v.RuleID, v.Message, tt.wantInMessage)
					}
				}
			}
		})
	}
}

// licenseRoot returns a temp root containing a license file at rel. An empty
// content creates the file with zero bytes.
func licenseRoot(t *testing.T, rel string, content ...string) string {
	t.Helper()
	root := t.TempDir()
	text := validMITText
	if len(content) == 1 {
		text = content[0]
	}
	writeFile(t, root, rel, text)
	return root
}

func formatViolations(vs []Violation) string {
	var b strings.Builder
	for _, v := range vs {
		b.WriteString("  [" + v.RuleID + "] " + v.Message + "\n")
	}
	return b.String()
}

// TestProvenanceRealManifest is the happy path over the repository's real
// manifest and the real LICENSES files it references.
func TestProvenanceRealManifest(t *testing.T) {
	root := filepath.Join("..", "..")
	raw, err := os.ReadFile(filepath.Join(root, "docs", "upstream", "provenance.yaml"))
	if err != nil {
		t.Fatalf("read real manifest: %v", err)
	}

	violations := ValidateProvenance(root, raw)
	if len(violations) != 0 {
		t.Errorf("real manifest has violations:\n%s", formatViolations(violations))
	}

	manifest, err := ParseProvenanceManifest(raw)
	if err != nil {
		t.Fatalf("parse real manifest: %v", err)
	}
	if len(manifest.Sources) != 2 {
		t.Fatalf("real manifest has %d sources, want 2", len(manifest.Sources))
	}
	pinned := map[string]string{
		"shadcn-admin": "e16c87f213a5ba5e45964e9b67c792105ec74d26",
		"go-admin":     "1b7dcd843ce38fddc8c280fe3139e02735cf7574",
	}
	seen := map[string]bool{}
	for _, s := range manifest.Sources {
		seen[s.Name] = true
		wantCommit, ok := pinned[s.Name]
		if !ok {
			t.Errorf("unexpected source %q in real manifest", s.Name)
			continue
		}
		if s.Commit != wantCommit {
			t.Errorf("source %q commit = %q, want pinned %q", s.Name, s.Commit, wantCommit)
		}
		if s.LicenseFile != "LICENSES/"+s.Name+"-MIT.txt" {
			t.Errorf("source %q license_file = %q", s.Name, s.LicenseFile)
		}
	}
	for name := range pinned {
		if !seen[name] {
			t.Errorf("real manifest is missing source %q", name)
		}
	}
}
