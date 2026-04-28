package static_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/thoth-go/thoth-go/internal/checker"
	"github.com/thoth-go/thoth-go/internal/checker/static"
)

// writeGoFile creates a temporary directory containing main.go with the given source.
func writeGoFile(t *testing.T, src string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(src), 0o644); err != nil {
		t.Fatalf("writeGoFile: %v", err)
	}
	return dir
}

// cfg builds a minimal ExerciseConfig with only the fields the test cares about.
func cfg(mode checker.Mode) *checker.ExerciseConfig {
	return &checker.ExerciseConfig{
		ID:    "test-001",
		Title: "Test",
		Topic: "test",
		Mode:  mode,
	}
}

// ── Import rule tests ─────────────────────────────────────────────────────────

func TestAnalyzeDir_BannedImport(t *testing.T) {
	src := `package main
import (
	"fmt"
	"os/exec"
)
func main() { fmt.Println("hi"); _ = exec.Command("ls") }
`
	dir := writeGoFile(t, src)
	c := cfg(checker.ModeExecutable)
	c.Imports.Banned = []string{"os/exec"}

	violations, err := static.New(c).AnalyzeDir(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !hasRule(violations, "banned_import") {
		t.Errorf("expected banned_import violation, got: %v", violations)
	}
}

func TestAnalyzeDir_AllowedImportOnly(t *testing.T) {
	src := `package main
import (
	"fmt"
	"os"
)
func main() { fmt.Println(os.Args) }
`
	dir := writeGoFile(t, src)
	c := cfg(checker.ModeExecutable)
	c.Imports.Allowed = []string{"fmt"} // "os" is not allowed

	violations, err := static.New(c).AnalyzeDir(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !hasRule(violations, "disallowed_import") {
		t.Errorf("expected disallowed_import violation, got: %v", violations)
	}
}

func TestAnalyzeDir_ImportRules_CleanSource(t *testing.T) {
	src := `package main
import "fmt"
func main() { fmt.Println("hello") }
`
	dir := writeGoFile(t, src)
	c := cfg(checker.ModeExecutable)
	c.Imports.Allowed = []string{"fmt"}
	c.Imports.Banned = []string{"os/exec"}

	violations, err := static.New(c).AnalyzeDir(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if importViolations := filterRule(violations, "banned_import", "disallowed_import"); len(importViolations) != 0 {
		t.Errorf("expected no import violations on clean source, got: %v", importViolations)
	}
}

// ── AST node tests ────────────────────────────────────────────────────────────

func TestAnalyzeDir_BannedForStmt(t *testing.T) {
	src := `package main
import "fmt"
func main() {
	for i := 0; i < 3; i++ {
		fmt.Println(i)
	}
}
`
	dir := writeGoFile(t, src)
	c := cfg(checker.ModeExecutable)
	c.BannedASTNodes = []string{"ast.ForStmt"}

	violations, err := static.New(c).AnalyzeDir(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !hasRule(violations, "banned_ast_node") {
		t.Errorf("expected banned_ast_node violation for ForStmt, got: %v", violations)
	}
}

func TestAnalyzeDir_BannedGoStmt(t *testing.T) {
	src := `package main
import "fmt"
func main() {
	go func() { fmt.Println("goroutine") }()
}
`
	dir := writeGoFile(t, src)
	c := cfg(checker.ModeExecutable)
	c.BannedASTNodes = []string{"ast.GoStmt"}

	violations, err := static.New(c).AnalyzeDir(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !hasRule(violations, "banned_ast_node") {
		t.Errorf("expected banned_ast_node violation for GoStmt, got: %v", violations)
	}
}

func TestAnalyzeDir_NoBannedNodes_CleanSource(t *testing.T) {
	src := `package main
import "fmt"
func main() { fmt.Println("ok") }
`
	dir := writeGoFile(t, src)
	c := cfg(checker.ModeExecutable)
	c.BannedASTNodes = []string{"ast.ForStmt"}

	violations, err := static.New(c).AnalyzeDir(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hasRule(violations, "banned_ast_node") {
		t.Errorf("unexpected banned_ast_node violation on clean source")
	}
}

// ── Required function tests ───────────────────────────────────────────────────

func TestCheckRequiredFunctions_Present(t *testing.T) {
	src := `package main
func Add(a, b int) int { return a + b }
func main() {}
`
	dir := writeGoFile(t, src)
	c := cfg(checker.ModeFunctionSignature)
	c.RequiredFunctions = []checker.FunctionSpec{
		{Name: "Add", Signature: "func Add(a, b int) int"},
	}

	violations, err := static.New(c).CheckRequiredFunctions(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(violations) != 0 {
		t.Errorf("expected no violations, got: %v", violations)
	}
}

func TestCheckRequiredFunctions_Missing(t *testing.T) {
	src := `package main
func main() {}
`
	dir := writeGoFile(t, src)
	c := cfg(checker.ModeFunctionSignature)
	c.RequiredFunctions = []checker.FunctionSpec{
		{Name: "Fibonacci", Signature: "func Fibonacci(n int) int"},
	}

	violations, err := static.New(c).CheckRequiredFunctions(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !hasRule(violations, "missing_function") {
		t.Errorf("expected missing_function violation, got: %v", violations)
	}
}

func TestCheckRequiredFunctions_MultiplePartiallyMissing(t *testing.T) {
	src := `package main
func Add(a, b int) int { return a + b }
func main() {}
`
	dir := writeGoFile(t, src)
	c := cfg(checker.ModeFunctionSignature)
	c.RequiredFunctions = []checker.FunctionSpec{
		{Name: "Add", Signature: "func Add(a, b int) int"},
		{Name: "Sub", Signature: "func Sub(a, b int) int"},
	}

	violations, err := static.New(c).CheckRequiredFunctions(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(violations) != 1 {
		t.Errorf("expected 1 violation for Sub, got %d: %v", len(violations), violations)
	}
}

// ── Violation.String tests ────────────────────────────────────────────────────

func TestViolation_String_WithPosition(t *testing.T) {
	v := static.Violation{
		Rule:    "banned_import",
		Message: "import \"os/exec\" is banned",
	}
	s := v.String()
	if s == "" {
		t.Error("String() must not be empty")
	}
}

// ── Helper functions ──────────────────────────────────────────────────────────

func hasRule(violations []static.Violation, rules ...string) bool {
	for _, v := range violations {
		for _, r := range rules {
			if v.Rule == r {
				return true
			}
		}
	}
	return false
}

func filterRule(violations []static.Violation, rules ...string) []static.Violation {
	want := make(map[string]bool)
	for _, r := range rules {
		want[r] = true
	}
	var out []static.Violation
	for _, v := range violations {
		if want[v.Rule] {
			out = append(out, v)
		}
	}
	return out
}
