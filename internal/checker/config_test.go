package checker
package checker_test

import (
	"strings"
	"testing"

	"github.com/thoth-go/thoth-go/internal/checker"
)

// minimalValid returns a YAML string that satisfies all required fields.
func minimalValid(overrides ...string) string {
	base := `
id: "test-001"
title: "Test Exercise"
topic: "basics"
mode: executable
test_cases:
  - args: []
    expected_stdout: "hello\n"
    expected_exit_code: 0
`
	return base + strings.Join(overrides, "\n")
}

func TestParseExerciseConfig_MinimalValid(t *testing.T) {
	cfg, err := checker.ParseExerciseConfig([]byte(minimalValid()))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.ID != "test-001" {
		t.Errorf("ID = %q, want test-001", cfg.ID)
	}
	if cfg.Mode != checker.ModeExecutable {
		t.Errorf("Mode = %q, want executable", cfg.Mode)
	}
	if len(cfg.TestCases) != 1 {
		t.Errorf("TestCases len = %d, want 1", len(cfg.TestCases))
	}
}

func TestParseExerciseConfig_AllFields(t *testing.T) {
	yaml := `
id: "concurrency-002"
title: "WaitGroups"
topic: "concurrency"
mode: function_signature
difficulty: 3
imports:
  allowed: ["fmt", "sync"]
  banned: ["os/exec"]
banned_ast_nodes:
  - "ast.GoStmt"
banned_functions:
  - "fmt.Println"
required_functions:
  - name: "RunWorkers"
    signature: "func RunWorkers(n int) []string"
test_cases:
  - args: ["5"]
    expected_stdout: "done\n"
    expected_exit_code: 0
    hidden: false
    description: "basic run"
  - args: ["10"]
    expected_stdout: "done\n"
    expected_exit_code: 0
    hidden: true
`
	cfg, err := checker.ParseExerciseConfig([]byte(yaml))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Difficulty != 3 {
		t.Errorf("Difficulty = %d, want 3", cfg.Difficulty)
	}
	if len(cfg.Imports.Allowed) != 2 {
		t.Errorf("Imports.Allowed len = %d, want 2", len(cfg.Imports.Allowed))
	}
	if cfg.Imports.Banned[0] != "os/exec" {
		t.Errorf("Imports.Banned[0] = %q, want os/exec", cfg.Imports.Banned[0])
	}
	if cfg.BannedASTNodes[0] != "ast.GoStmt" {
		t.Errorf("BannedASTNodes[0] = %q, want ast.GoStmt", cfg.BannedASTNodes[0])
	}
	if cfg.RequiredFunctions[0].Name != "RunWorkers" {
		t.Errorf("RequiredFunctions[0].Name = %q", cfg.RequiredFunctions[0].Name)
	}
	hidden := 0
	for _, tc := range cfg.TestCases {
		if tc.Hidden {
			hidden++
		}
	}
	if hidden != 1 {
		t.Errorf("hidden test cases = %d, want 1", hidden)
	}
}

func TestParseExerciseConfig_BugFixMode(t *testing.T) {
	yaml := `
id: "bugfix-001"
title: "Fix the off-by-one"
topic: "debugging"
mode: bug_fix
bug_fix:
  reference_file: "solution.go"
  restricted_lines: [10, 11, 12]
test_cases:
  - args: []
    expected_stdout: ""
    expected_exit_code: 0
`
	cfg, err := checker.ParseExerciseConfig([]byte(yaml))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.BugFix.ReferenceFile != "solution.go" {
		t.Errorf("BugFix.ReferenceFile = %q", cfg.BugFix.ReferenceFile)
	}
	if len(cfg.BugFix.RestrictedLines) != 3 {
		t.Errorf("RestrictedLines len = %d, want 3", len(cfg.BugFix.RestrictedLines))
	}
}

func TestParseExerciseConfig_ValidationErrors(t *testing.T) {
	cases := []struct {
		name    string
		yaml    string
		wantErr string
	}{
		{
			name:    "missing id",
			yaml:    "title: T\ntopic: t\nmode: executable\n",
			wantErr: "'id' is required",
		},
		{
			name:    "missing title",
			yaml:    "id: x\ntopic: t\nmode: executable\n",
			wantErr: "'title' is required",
		},
		{
			name:    "missing topic",
			yaml:    "id: x\ntitle: T\nmode: executable\n",
			wantErr: "'topic' is required",
		},
		{
			name:    "missing mode",
			yaml:    "id: x\ntitle: T\ntopic: t\n",
			wantErr: "'mode' is required",
		},
		{
			name:    "unknown mode",
			yaml:    "id: x\ntitle: T\ntopic: t\nmode: magic\n",
			wantErr: "unknown mode",
		},
		{
			name:    "bug_fix missing reference_file",
			yaml:    "id: x\ntitle: T\ntopic: t\nmode: bug_fix\n",
			wantErr: "reference_file",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := checker.ParseExerciseConfig([]byte(tc.yaml))
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error %q does not contain %q", err.Error(), tc.wantErr)
			}
		})
	}
}

func TestParseExerciseConfig_UnknownField(t *testing.T) {
	yaml := minimalValid() + "\nunknown_key: oops\n"
	_, err := checker.ParseExerciseConfig([]byte(yaml))
	if err == nil {
		t.Fatal("expected error for unknown field, got nil")
	}
}

func TestParseExerciseConfig_StdinField(t *testing.T) {
	yaml := `
id: "stdin-001"
title: "Echo stdin"
topic: "io"
mode: executable
test_cases:
  - args: []
    stdin: "hello world\n"
    expected_stdout: "hello world\n"
    expected_exit_code: 0
`
	cfg, err := checker.ParseExerciseConfig([]byte(yaml))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.TestCases[0].Stdin != "hello world\n" {
		t.Errorf("Stdin = %q", cfg.TestCases[0].Stdin)
	}
}
