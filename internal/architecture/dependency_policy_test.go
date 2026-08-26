package architecture

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Dependency policy fixtures (Issue #105 Task 2). The fixture tree lives
// under testdata/dependency-policy: the Go toolchain never compiles it, and
// the scanner under test exempts any path with a testdata segment when
// scanning from the repository root — so the fixture-root tests below scan
// the fixture directory AS the root, where relative paths carry no testdata
// segment and every fixture is visible to the rules.
const dependencyPolicyFixtureRoot = "testdata/dependency-policy"

// fixtureExemptions exempts the exemption fixture so the plain rule tests
// observe exactly the violations their tables describe.
func fixtureExemptions() []string { return []string{"exempted/server.gen.go"} }

func scanDependencyPolicyFixtures(t *testing.T, exemptions []string) []Violation {
	t.Helper()
	vs, err := RunDependencyPolicy(dependencyPolicyFixtureRoot, exemptions)
	if err != nil {
		t.Fatalf("RunDependencyPolicy(%q, %v): %v", dependencyPolicyFixtureRoot, exemptions, err)
	}
	return vs
}

func TestDependencyPolicyImportRuleFixtures(t *testing.T) {
	vs := scanDependencyPolicyFixtures(t, fixtureExemptions())

	cases := []struct {
		file       string
		ruleID     string
		importPath string
	}{
		{"import_gorm.go", RuleArchGorm, "gorm.io/gorm"},
		{"import_casbin.go", RuleArchCasbin, "github.com/casbin/casbin/v2"},
		{"import_swaggo.go", RuleArchSwaggo, "github.com/swaggo/swag"},
		{"import_go_admin.go", RuleArchGoAdmin, "github.com/go-admin-team/go-admin/common/global"},
		{"import_jwt_golang.go", RuleArchUpstreamJWT, "github.com/golang-jwt/jwt/v4"},
		{"import_jwt_dgrijalva.go", RuleArchUpstreamJWT, "github.com/dgrijalva/jwt-go"},
	}
	importRules := map[string]bool{
		RuleArchGorm: true, RuleArchCasbin: true, RuleArchSwaggo: true,
		RuleArchGoAdmin: true, RuleArchUpstreamJWT: true,
	}
	importViolations := 0
	for _, v := range vs {
		if importRules[v.RuleID] {
			importViolations++
		}
	}
	if importViolations != len(cases) {
		t.Fatalf("want exactly %d import-rule violations, got %d: %v", len(cases), importViolations, vs)
	}
	for _, tc := range cases {
		found := false
		for _, v := range vs {
			if v.RuleID == tc.ruleID && strings.Contains(v.Message, tc.file+":") && strings.Contains(v.Message, `"`+tc.importPath+`"`) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("no %s violation reporting file %q with import %q; violations: %v", tc.ruleID, tc.file, tc.importPath, vs)
		}
	}
}

func TestDependencyPolicyContentRuleFixtures(t *testing.T) {
	vs := scanDependencyPolicyFixtures(t, fixtureExemptions())

	cases := []struct {
		file   string
		ruleID string
	}{
		{"content_default_credentials.go", RuleArchDefaultCredentials},
		{"content_host_tenant.go", RuleArchHostDerivedTenantID},
	}
	for _, tc := range cases {
		found := false
		for _, v := range vs {
			if v.RuleID == tc.ruleID && strings.Contains(v.Message, tc.file+":") {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("no %s violation reporting file %q; violations: %v", tc.ruleID, tc.file, vs)
		}
	}
}

// TestDependencyPolicyExemptions verifies the parameterized exemption logic
// on a controlled temporary root (design ruling 5): explicit generated-file
// paths are exempt, and any directory named testdata is exempt everywhere.
func TestDependencyPolicyExemptions(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "generated/server.gen.go", "package generated\n\nimport _ \"gorm.io/gorm\"\n")
	writeFile(t, root, "nested/testdata/legacy.go", "package legacy\n\nimport _ \"gorm.io/gorm\"\n")
	writeFile(t, root, "plain/plug.go", "package plain\n\nimport _ \"gorm.io/gorm\"\n")

	t.Run("exempt path is skipped", func(t *testing.T) {
		vs, err := RunDependencyPolicy(root, []string{"generated/server.gen.go"})
		if err != nil {
			t.Fatalf("RunDependencyPolicy: %v", err)
		}
		if len(vs) != 1 || !strings.Contains(vs[0].Message, "plain/plug.go") || vs[0].RuleID != RuleArchGorm {
			t.Fatalf("want exactly one ARCH-GORM violation on plain/plug.go, got %v", vs)
		}
	})
	t.Run("without the exemption the file is flagged", func(t *testing.T) {
		vs, err := RunDependencyPolicy(root, nil)
		if err != nil {
			t.Fatalf("RunDependencyPolicy: %v", err)
		}
		flagged := 0
		for _, v := range vs {
			if strings.Contains(v.Message, "generated/server.gen.go") {
				flagged++
			}
		}
		if flagged == 0 {
			t.Fatalf("generated/server.gen.go must be flagged when not exempt; violations: %v", vs)
		}
	})
	t.Run("testdata segment is always exempt", func(t *testing.T) {
		for _, exemptions := range [][]string{nil, {"generated/server.gen.go"}} {
			vs, err := RunDependencyPolicy(root, exemptions)
			if err != nil {
				t.Fatalf("RunDependencyPolicy(%v): %v", exemptions, err)
			}
			for _, v := range vs {
				if strings.Contains(v.Message, "nested/testdata/") {
					t.Fatalf("testdata path must never be flagged; violations: %v", vs)
				}
			}
		}
	})
	t.Run("exemption fixture under the fixture root", func(t *testing.T) {
		vs := scanDependencyPolicyFixtures(t, nil)
		flagged := false
		for _, v := range vs {
			if strings.Contains(v.Message, "exempted/server.gen.go") {
				flagged = true
			}
		}
		if !flagged {
			t.Fatalf("exempted/server.gen.go must be flagged when no exemption is passed; violations: %v", vs)
		}
		vs = scanDependencyPolicyFixtures(t, fixtureExemptions())
		for _, v := range vs {
			if strings.Contains(v.Message, "exempted/server.gen.go") {
				t.Fatalf("exempted/server.gen.go flagged despite the exemption; violations: %v", vs)
			}
		}
	})
}

// TestDependencyPolicyTestFileScoping pins the file-class scoping of design
// ruling 3/4: import rules apply to _test.go files too, while content rules
// scan production files only.
func TestDependencyPolicyTestFileScoping(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "probe/probe_test.go",
		"package probe\n\nimport _ \"gorm.io/gorm\"\n\nconst pin = \"123456\"\n")
	writeFile(t, root, "probe/probe.go",
		"package probe\n\nconst fallback = \"123456\"\n")

	vs, err := RunDependencyPolicy(root, nil)
	if err != nil {
		t.Fatalf("RunDependencyPolicy: %v", err)
	}
	var testImport, testContent, prodContent int
	for _, v := range vs {
		switch {
		case v.RuleID == RuleArchGorm && strings.Contains(v.Message, "probe/probe_test.go"):
			testImport++
		case v.RuleID == RuleArchDefaultCredentials && strings.Contains(v.Message, "probe/probe_test.go"):
			testContent++
		case v.RuleID == RuleArchDefaultCredentials && strings.Contains(v.Message, "probe/probe.go"):
			prodContent++
		}
	}
	if testImport != 1 {
		t.Errorf("import rules must scan _test.go files: want 1 ARCH-GORM in probe_test.go, got %d (%v)", testImport, vs)
	}
	if testContent != 0 {
		t.Errorf("content rules must skip _test.go files: got %d ARCH-DEFAULT-CREDENTIALS in probe_test.go (%v)", testContent, vs)
	}
	if prodContent == 0 {
		t.Errorf("content rules must scan production files: no ARCH-DEFAULT-CREDENTIALS in probe.go (%v)", vs)
	}
}

// TestDependencyPolicyRepoScan is the repository gate behind
// `make policy-check`: the real tree must scan clean under the fixed
// generated-file exemption list.
func TestDependencyPolicyRepoScan(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	root := filepath.Join(cwd, "..", "..")

	// Anti-vacuous guard: refuse a green result from a wrong root.
	for _, mustExist := range GeneratedFileExemptions() {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(mustExist))); err != nil {
			t.Fatalf("repo root sanity check failed for %s: %v", mustExist, err)
		}
	}

	vs, err := RunDependencyPolicy(root, GeneratedFileExemptions())
	if err != nil {
		t.Fatalf("RunDependencyPolicy(repo): %v", err)
	}
	if len(vs) != 0 {
		t.Fatalf("real repository must scan clean, got %d violation(s):\n%s", len(vs), joinViolations(vs))
	}
}

// TestDependencyPolicyRefusesVacuousRoot pins the anti-green-drift behavior:
// a root with no scannable .go files is an error, never a clean scan.
func TestDependencyPolicyRefusesVacuousRoot(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "notes/readme.txt", "not go source\n")
	if _, err := RunDependencyPolicy(root, nil); err == nil {
		t.Fatal("scanning a root with zero .go files must return an error, got nil")
	}
}

func joinViolations(vs []Violation) string {
	var b strings.Builder
	for _, v := range vs {
		b.WriteString(v.String())
		b.WriteString("\n")
	}
	return b.String()
}
