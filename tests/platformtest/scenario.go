package platformtest

type Scenario struct {
	ID         string         `yaml:"id" json:"id"`
	Seam       string         `yaml:"seam" json:"seam"`
	Timeout    string         `yaml:"timeout,omitempty" json:"timeout,omitempty"`
	Inputs     map[string]any `yaml:"inputs,omitempty" json:"inputs,omitempty"`
	Assertions []Assertion    `yaml:"assertions,omitempty" json:"assertions,omitempty"`
	Metadata   map[string]any `yaml:"metadata,omitempty" json:"metadata,omitempty"`
}

type Assertion struct {
	Name string         `yaml:"name" json:"name"`
	Want map[string]any `yaml:"want,omitempty" json:"want,omitempty"`
}

type Report struct {
	ScenarioID    string            `json:"scenario_id"`
	Seam          string            `json:"seam,omitempty"`
	Passed        bool              `json:"passed"`
	Summary       string            `json:"summary"`
	Assertions    []AssertionResult `json:"assertions,omitempty"`
	StartedAt     string            `json:"started_at"`
	FinishedAt    string            `json:"finished_at"`
	Duration      string            `json:"duration"`
	Revision      string            `json:"revision,omitempty"`
	Dependencies  map[string]string `json:"dependencies,omitempty"`
	Scenario      map[string]any    `json:"scenario,omitempty"`
	EvidencePath  string            `json:"evidence_path,omitempty"`
	FailureReason string            `json:"failure_reason,omitempty"`
}

type AssertionResult struct {
	Name    string `json:"name"`
	Passed  bool   `json:"passed"`
	Details string `json:"details,omitempty"`
}
