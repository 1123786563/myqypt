// Package architecture hosts repository-level gate tooling for the F05
// foundation (Issue #105). It observes sources only: nothing here is runtime
// code, and no package outside internal/architecture may import it in
// non-test files.
package architecture

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// Stable provenance rule IDs (Issue #105 implementation plan, design ruling 2).
const (
	RuleProvSchemaVersion      = "PROV-SCHEMA-VERSION"
	RuleProvSourceEmpty        = "PROV-SOURCE-EMPTY"
	RuleProvName               = "PROV-NAME"
	RuleProvRepository         = "PROV-REPOSITORY"
	RuleProvCommit             = "PROV-COMMIT"
	RuleProvLicense            = "PROV-LICENSE"
	RuleProvLicenseFileMissing = "PROV-LICENSE-FILE-MISSING"
	RuleProvLicenseFileEmpty   = "PROV-LICENSE-FILE-EMPTY"
	RuleProvLicenseText        = "PROV-LICENSE-TEXT"
	RuleProvMode               = "PROV-MODE"
	RuleProvDestForbiddenMode  = "PROV-DEST-FORBIDDEN-MODE"
	RuleProvDestMissing        = "PROV-DEST-MISSING"
	RuleProvDestOutsideRepo    = "PROV-DEST-OUTSIDE-REPO"
	RuleProvDestEmpty          = "PROV-DEST-EMPTY"
	RuleProvModificationsEmpty = "PROV-MODIFICATIONS-EMPTY"
)

// Extraction modes (Issue #105 implementation plan, design ruling 1).
const (
	// ModeVerbatim marks sources whose files were copied byte-for-byte; their
	// destinations must be declared and must exist inside the repository.
	ModeVerbatim = "verbatim"
	// ModePatternOnly marks sources from which only engineering ideas were
	// adopted; destinations must stay empty and local modifications must be
	// recorded.
	ModePatternOnly = "pattern-only"
)

// commitPattern matches exactly 40 lowercase hexadecimal characters.
var commitPattern = regexp.MustCompile(`^[0-9a-f]{40}$`)

// ProvenanceManifest is the parsed form of docs/upstream/provenance.yaml.
type ProvenanceManifest struct {
	SchemaVersion int                `yaml:"schema_version"`
	Sources       []ProvenanceSource `yaml:"sources"`
}

// ProvenanceSource records one upstream extraction source.
type ProvenanceSource struct {
	Name               string   `yaml:"name"`
	Repository         string   `yaml:"repository"`
	Commit             string   `yaml:"commit"`
	License            string   `yaml:"license"`
	LicenseFile        string   `yaml:"license_file"`
	ExtractionMode     string   `yaml:"extraction_mode"`
	Destinations       []string `yaml:"destinations"`
	LocalModifications []string `yaml:"local_modifications"`
}

// Violation is one failed provenance rule check. Message always names the
// offending source (or manifest location) so the failure can be traced.
type Violation struct {
	RuleID  string
	Message string
}

func (v Violation) String() string { return v.RuleID + ": " + v.Message }

// ParseProvenanceManifest parses manifest YAML into a ProvenanceManifest.
func ParseProvenanceManifest(data []byte) (*ProvenanceManifest, error) {
	var m ProvenanceManifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

// ValidateProvenance validates manifest YAML (the content of
// docs/upstream/provenance.yaml) against the PROV-* rule set. root is the
// repository root used to resolve license_file and destination paths; it is a
// parameter so tests can inject temporary roots.
func ValidateProvenance(root string, manifestYAML []byte) []Violation {
	manifest, err := ParseProvenanceManifest(manifestYAML)
	if err != nil {
		return []Violation{{
			RuleID:  RuleProvSchemaVersion,
			Message: fmt.Sprintf("manifest does not parse against schema_version 1: %v", err),
		}}
	}

	var vs []Violation
	if manifest.SchemaVersion != 1 {
		vs = append(vs, Violation{
			RuleID:  RuleProvSchemaVersion,
			Message: fmt.Sprintf("schema_version must be 1, got %d", manifest.SchemaVersion),
		})
	}
	if len(manifest.Sources) == 0 {
		vs = append(vs, Violation{
			RuleID:  RuleProvSourceEmpty,
			Message: "sources must declare at least one upstream",
		})
	}

	seenNames := map[string]bool{}
	for i := range manifest.Sources {
		s := manifest.Sources[i]
		if s.Name == "" {
			vs = append(vs, Violation{
				RuleID:  RuleProvName,
				Message: fmt.Sprintf("source[%d]: name must not be empty", i),
			})
		} else if seenNames[s.Name] {
			vs = append(vs, Violation{
				RuleID:  RuleProvName,
				Message: fmt.Sprintf("source %q (index %d): duplicate name", s.Name, i),
			})
		} else {
			seenNames[s.Name] = true
		}

		vs = append(vs, validateSource(root, i, s)...)
	}
	return vs
}

// validateSource checks one source and prefixes every message with its
// location: `source "name"` when the name is usable, `source[i]` otherwise.
func validateSource(root string, index int, s ProvenanceSource) []Violation {
	loc := fmt.Sprintf("source[%d]", index)
	if s.Name != "" {
		loc = fmt.Sprintf("source %q", s.Name)
	}

	var vs []Violation
	if u, err := url.Parse(s.Repository); err != nil || u.Scheme != "https" || u.Host == "" {
		vs = append(vs, Violation{
			RuleID:  RuleProvRepository,
			Message: fmt.Sprintf("%s: repository %q must be an https URL", loc, s.Repository),
		})
	}
	if !commitPattern.MatchString(s.Commit) {
		vs = append(vs, Violation{
			RuleID:  RuleProvCommit,
			Message: fmt.Sprintf("%s: commit %q must be 40 lowercase hex characters", loc, s.Commit),
		})
	}
	if s.License != "MIT" {
		vs = append(vs, Violation{
			RuleID:  RuleProvLicense,
			Message: fmt.Sprintf("%s: license %q is not the SPDX identifier %q", loc, s.License, "MIT"),
		})
	}

	vs = append(vs, validateLicenseFile(root, loc, s)...)

	switch s.ExtractionMode {
	case ModePatternOnly:
		if len(s.Destinations) > 0 {
			vs = append(vs, Violation{
				RuleID:  RuleProvDestForbiddenMode,
				Message: fmt.Sprintf("%s: pattern-only extraction must not declare destinations, got %d (%s)", loc, len(s.Destinations), strings.Join(s.Destinations, ", ")),
			})
		}
		if len(s.LocalModifications) == 0 {
			vs = append(vs, Violation{
				RuleID:  RuleProvModificationsEmpty,
				Message: fmt.Sprintf("%s: pattern-only extraction must record non-empty local_modifications", loc),
			})
		}
	case ModeVerbatim:
		if len(s.Destinations) == 0 {
			vs = append(vs, Violation{
				RuleID:  RuleProvDestEmpty,
				Message: fmt.Sprintf("%s: verbatim extraction must declare at least one destination", loc),
			})
		}
		for _, dest := range s.Destinations {
			if pathEscapesRepo(dest) {
				vs = append(vs, Violation{
					RuleID:  RuleProvDestOutsideRepo,
					Message: fmt.Sprintf("%s: destination %q must be a repository-relative path without %q", loc, dest, ".."),
				})
				continue
			}
			if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(dest))); err != nil {
				vs = append(vs, Violation{
					RuleID:  RuleProvDestMissing,
					Message: fmt.Sprintf("%s: destination %q does not exist under repo root %q", loc, dest, root),
				})
			}
		}
	default:
		vs = append(vs, Violation{
			RuleID:  RuleProvMode,
			Message: fmt.Sprintf("%s: extraction_mode %q must be %q or %q", loc, s.ExtractionMode, ModeVerbatim, ModePatternOnly),
		})
	}
	return vs
}

// validateLicenseFile enforces PROV-LICENSE-FILE-MISSING,
// PROV-LICENSE-FILE-EMPTY, and PROV-LICENSE-TEXT for one source.
func validateLicenseFile(root, loc string, s ProvenanceSource) []Violation {
	if s.LicenseFile == "" {
		return []Violation{{
			RuleID:  RuleProvLicenseFileMissing,
			Message: fmt.Sprintf("%s: license_file is not set", loc),
		}}
	}
	if pathEscapesRepo(s.LicenseFile) {
		return []Violation{{
			RuleID:  RuleProvLicenseFileMissing,
			Message: fmt.Sprintf("%s: license_file %q must be a repository-relative path without %q", loc, s.LicenseFile, ".."),
		}}
	}
	path := filepath.Join(root, filepath.FromSlash(s.LicenseFile))
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return []Violation{{
			RuleID:  RuleProvLicenseFileMissing,
			Message: fmt.Sprintf("%s: license file %q not found under repo root %q", loc, s.LicenseFile, root),
		}}
	}
	if info.Size() == 0 {
		return []Violation{{
			RuleID:  RuleProvLicenseFileEmpty,
			Message: fmt.Sprintf("%s: license file %q is empty", loc, s.LicenseFile),
		}}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return []Violation{{
			RuleID:  RuleProvLicenseFileMissing,
			Message: fmt.Sprintf("%s: license file %q cannot be read: %v", loc, s.LicenseFile, err),
		}}
	}
	var missing []string
	for _, needle := range []string{"MIT License", "Copyright"} {
		if !strings.Contains(string(data), needle) {
			missing = append(missing, fmt.Sprintf("%q", needle))
		}
	}
	if len(missing) > 0 {
		return []Violation{{
			RuleID:  RuleProvLicenseText,
			Message: fmt.Sprintf("%s: license file %q must contain %q and %q, missing %s", loc, s.LicenseFile, "MIT License", "Copyright", strings.Join(missing, " and ")),
		}}
	}
	return nil
}

// pathEscapesRepo reports whether a slash-separated destination is absolute
// or contains a ".." path element.
func pathEscapesRepo(dest string) bool {
	if filepath.IsAbs(dest) || strings.HasPrefix(dest, "/") {
		return true
	}
	for _, part := range strings.Split(dest, "/") {
		if part == ".." {
			return true
		}
	}
	return false
}
