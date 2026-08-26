package architecture

import (
	"fmt"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Stable dependency-policy rule IDs (Issue #105 implementation plan, design
// rulings 3 and 4). The import rules forbid upstream heavy dependencies the
// extraction design rejected (GORM, Casbin, Swaggo, the whole go-admin
// module including its common/global runtime, and upstream JWT libraries);
// the content rules forbid seeded default credentials and host-derived
// tenant resolution in production sources.
const (
	RuleArchGorm                = "ARCH-GORM"
	RuleArchCasbin              = "ARCH-CASBIN"
	RuleArchSwaggo              = "ARCH-SWAGGO"
	RuleArchGoAdmin             = "ARCH-GO-ADMIN"
	RuleArchUpstreamJWT         = "ARCH-UPSTREAM-JWT"
	RuleArchDefaultCredentials  = "ARCH-DEFAULT-CREDENTIALS"
	RuleArchHostDerivedTenantID = "ARCH-HOST-TENANT" // identifier deliberately avoids the ruled pattern so this file stays self-clean
)

// importRulePrefixes maps forbidden import-path prefixes to rule IDs (design
// ruling 3). Matching is element-boundary prefix matching: the import path
// equals the prefix or continues with "/".
var importRulePrefixes = []struct {
	Prefix string
	RuleID string
}{
	{"gorm.io/gorm", RuleArchGorm},
	{"github.com/casbin", RuleArchCasbin},
	{"github.com/swaggo", RuleArchSwaggo},
	{"github.com/go-admin-team/go-admin", RuleArchGoAdmin},
	{"github.com/golang-jwt", RuleArchUpstreamJWT},
	{"github.com/dgrijalva/jwt-go", RuleArchUpstreamJWT},
}

// GeneratedFileExemptions is the fixed generated-file exemption list (design
// ruling 5): exactly one entry, the oapi-codegen output. Everything else in
// the tree is scanned.
func GeneratedFileExemptions() []string {
	return []string{"internal/transport/http/api/server.gen.go"}
}

// Content-rule data (design ruling 4). This file is itself a production file
// scanned by these very rules, so every pattern literal is assembled from
// concatenated fragments: no matchable string appears contiguously in this
// source, which keeps the gate self-clean without inventing exemptions.
var (
	// Substring tokens and identifier names for ARCH-DEFAULT-CREDENTIALS.
	defaultCredentialTokens = []string{"admin" + "123", "123" + "456", "password" + "123"}
	defaultCredentialIDs    = []string{"Default" + "Admin", "Default" + "Password"}
	// Case-insensitive phrases (stored lowercase) for ARCH-DEFAULT-CREDENTIALS.
	defaultCredentialPhrases = []string{"default" + " admin", "default" + " password", "默认" + "管理员", "默认" + "密码"}
	// Identifier pattern and case-insensitive phrase for ARCH-HOST-TENANT.
	tenantHostPattern = regexp.MustCompile("[Tt]enant" + "FromHost|[Hh]ost" + "[Tt]enant")
	tenantHostPhrases = []string{"host" + "-based tenant"}
)

// skippedDirNames mirrors Go toolchain directory semantics for walking
// sources: testdata directories are never compiled, and directories starting
// with "." or "_" are ignored by go tooling. node_modules is a JS dependency
// tree, not authored Go source, and is skipped for the same reason.
func skippedDirName(name string) bool {
	return strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") ||
		name == "testdata" || name == "node_modules"
}

// RunDependencyPolicy scans the Go sources under root for forbidden imports
// (ruling 3; all .go files including _test.go) and forbidden production-file
// content (ruling 4; every non-_test.go file), line by line. exemptions are
// root-relative slash paths of generated files exempt from both rule classes
// (ruling 5). A root containing no scannable .go file is an error, never a
// clean scan, so a misconfigured root cannot green the gate vacuously.
func RunDependencyPolicy(root string, exemptions []string) ([]Violation, error) {
	exempt := make(map[string]bool, len(exemptions))
	for _, e := range exemptions {
		exempt[filepath.ToSlash(e)] = true
	}

	scanned := 0
	var vs []Violation
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if rel != "." && skippedDirName(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".go") || exempt[rel] {
			return nil
		}
		scanned++
		fileVs, scanErr := scanGoFile(root, rel)
		if scanErr != nil {
			return scanErr
		}
		vs = append(vs, fileVs...)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("dependency policy scan of %q: %w", root, err)
	}
	if scanned == 0 {
		return nil, fmt.Errorf("dependency policy scan of %q found no .go files; refusing vacuous clean scan", root)
	}
	return vs, nil
}

// scanGoFile applies the import rules to one file and, for production files,
// the content rules. Every violation message starts with "rel:line:" so
// failures cite the offending file and line directly.
func scanGoFile(root, rel string) ([]Violation, error) {
	abs := filepath.Join(root, filepath.FromSlash(rel))
	src, err := os.ReadFile(abs)
	if err != nil {
		return nil, err
	}

	var vs []Violation

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, abs, src, parser.ImportsOnly)
	if err != nil {
		return nil, fmt.Errorf("%s: cannot parse imports: %w", rel, err)
	}
	for _, spec := range f.Imports {
		for _, rule := range importRulePrefixes {
			if spec.Path.Value == "" {
				continue
			}
			importPath := strings.Trim(spec.Path.Value, `"`)
			if importPath == rule.Prefix || strings.HasPrefix(importPath, rule.Prefix+"/") {
				vs = append(vs, Violation{
					RuleID: rule.RuleID,
					Message: fmt.Sprintf("%s:%d: import %q matches forbidden prefix %q (rule %s)",
						rel, fset.Position(spec.Pos()).Line, importPath, rule.Prefix, rule.RuleID),
				})
				break
			}
		}
	}

	if strings.HasSuffix(rel, "_test.go") {
		return vs, nil
	}

	lines := strings.Split(string(src), "\n")
	for i, line := range lines {
		lineNo := i + 1
		lower := strings.ToLower(line)
		for _, tok := range defaultCredentialTokens {
			if strings.Contains(line, tok) {
				vs = append(vs, credentialViolation(rel, lineNo, "substring", tok))
			}
		}
		for _, id := range defaultCredentialIDs {
			if strings.Contains(line, id) {
				vs = append(vs, credentialViolation(rel, lineNo, "identifier", id))
			}
		}
		for _, phrase := range defaultCredentialPhrases {
			if strings.Contains(lower, phrase) {
				vs = append(vs, credentialViolation(rel, lineNo, "phrase", phrase))
			}
		}
		if matched := tenantHostPattern.FindString(line); matched != "" {
			vs = append(vs, tenantHostViolation(rel, lineNo, "identifier", matched))
		}
		for _, phrase := range tenantHostPhrases {
			if strings.Contains(lower, phrase) {
				vs = append(vs, tenantHostViolation(rel, lineNo, "phrase", phrase))
			}
		}
	}
	return vs, nil
}

func credentialViolation(rel string, line int, kind, matched string) Violation {
	return Violation{
		RuleID: RuleArchDefaultCredentials,
		Message: fmt.Sprintf("%s:%d: %s %s %q indicates seeded default credentials (rule %s)",
			rel, line, RuleArchDefaultCredentials, kind, matched, RuleArchDefaultCredentials),
	}
}

func tenantHostViolation(rel string, line int, kind, matched string) Violation {
	return Violation{
		RuleID: RuleArchHostDerivedTenantID,
		Message: fmt.Sprintf("%s:%d: %s %s %q indicates host-derived tenant resolution (rule %s)",
			rel, line, RuleArchHostDerivedTenantID, kind, matched, RuleArchHostDerivedTenantID),
	}
}
