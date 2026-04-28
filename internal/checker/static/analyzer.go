// Package static implements the AST and go/types static analysis engine for
// thoth-go. All rules are driven by an ExerciseConfig — no hardcoded logic.
package static

import (
	"fmt"
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"strings"

	"github.com/thoth-go/thoth-go/internal/checker"
)

// Violation is a single rule violation discovered during static analysis.
type Violation struct {
	// Rule is the category of violation (e.g. "banned_import", "banned_ast_node").
	Rule string
	// Message is the human-readable explanation.
	Message string
	// Position is the source location (may be empty for import violations).
	Position token.Position
}

func (v Violation) String() string {
	if v.Position.IsValid() {
		return fmt.Sprintf("[%s] %s (%s)", v.Rule, v.Message, v.Position)
	}
	return fmt.Sprintf("[%s] %s", v.Rule, v.Message)
}

// Checker runs all static checks defined in an ExerciseConfig against a set of
// Go source files rooted at a directory.
type Checker struct {
	cfg *checker.ExerciseConfig
}

// New constructs a static Checker for the given exercise configuration.
func New(cfg *checker.ExerciseConfig) *Checker {
	return &Checker{cfg: cfg}
}

// AnalyzeDir parses all .go files in dir and returns any violations found.
// It applies (in order): import rules, banned AST nodes, banned functions.
func (c *Checker) AnalyzeDir(dir string) ([]Violation, error) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, dir, nil, parser.AllErrors)
	if err != nil {
		return nil, fmt.Errorf("static analysis: parsing %s: %w", dir, err)
	}

	// Collect all files across packages (exercises are always single-package).
	var files []*ast.File
	for _, pkg := range pkgs {
		for _, f := range pkg.Files {
			files = append(files, f)
		}
	}

	var violations []Violation

	// 1. Import rules
	violations = append(violations, c.checkImports(fset, files)...)

	// 2. Banned AST nodes
	violations = append(violations, c.checkASTNodes(fset, files)...)

	// 3. Banned functions (requires type-checking)
	if len(c.cfg.BannedFunctions) > 0 {
		typeViolations, err := c.checkBannedFunctions(fset, files, dir)
		if err != nil {
			// Type-checking errors are non-fatal: report as a warning violation.
			violations = append(violations, Violation{
				Rule:    "type_check_error",
				Message: fmt.Sprintf("type checking failed (results may be incomplete): %v", err),
			})
		} else {
			violations = append(violations, typeViolations...)
		}
	}

	return violations, nil
}

// ── Import rule checker ───────────────────────────────────────────────────────

func (c *Checker) checkImports(fset *token.FileSet, files []*ast.File) []Violation {
	var violations []Violation
	for _, f := range files {
		for _, imp := range f.Imports {
			path := strings.Trim(imp.Path.Value, `"`)
			pos := fset.Position(imp.Pos())

			// Banned list (checked regardless of allowlist).
			for _, banned := range c.cfg.Imports.Banned {
				if path == banned {
					violations = append(violations, Violation{
						Rule:     "banned_import",
						Message:  fmt.Sprintf("import %q is banned for this exercise", path),
						Position: pos,
					})
				}
			}

			// Allowlist (non-empty allowlist acts as a whitelist).
			if len(c.cfg.Imports.Allowed) > 0 && !contains(c.cfg.Imports.Allowed, path) {
				violations = append(violations, Violation{
					Rule:     "disallowed_import",
					Message:  fmt.Sprintf("import %q is not in the allowed list for this exercise", path),
					Position: pos,
				})
			}
		}
	}
	return violations
}

// ── Banned AST node checker ───────────────────────────────────────────────────

// astNodeType maps the string names from exercise.yaml to concrete ast.Node
// type names.  We compare using the ast package's reflect-based type names.
func (c *Checker) checkASTNodes(fset *token.FileSet, files []*ast.File) []Violation {
	banned := make(map[string]bool, len(c.cfg.BannedASTNodes))
	for _, n := range c.cfg.BannedASTNodes {
		banned[n] = true
	}
	if len(banned) == 0 {
		return nil
	}

	var violations []Violation
	for _, f := range files {
		ast.Inspect(f, func(n ast.Node) bool {
			if n == nil {
				return true
			}
			// fmt.Sprintf("%T", node) yields e.g. "*ast.ForStmt"; strip the pointer
			// to produce "ast.ForStmt" which matches exercise.yaml entries.
			typeName := strings.TrimPrefix(fmt.Sprintf("%T", n), "*")
			if banned[typeName] {
				violations = append(violations, Violation{
					Rule:     "banned_ast_node",
					Message:  fmt.Sprintf("%s is not allowed in this exercise", typeName),
					Position: fset.Position(n.Pos()),
				})
			}
			return true
		})
	}
	return violations
}

// ── Banned function checker (via go/types) ────────────────────────────────────

func (c *Checker) checkBannedFunctions(fset *token.FileSet, files []*ast.File, _ string) ([]Violation, error) {
	conf := types.Config{
		Importer: importer.Default(),
		Error:    func(err error) {}, // collect but don't abort
	}
	info := &types.Info{
		Uses: make(map[*ast.Ident]types.Object),
	}
	_, err := conf.Check("exercise", fset, files, info)
	if err != nil {
		// Non-fatal: return what we have.
		return c.scanUsages(info), err
	}
	return c.scanUsages(info), nil
}

func (c *Checker) scanUsages(info *types.Info) []Violation {
	var violations []Violation
	for ident, obj := range info.Uses {
		if obj == nil {
			continue
		}
		pkg := obj.Pkg()
		if pkg == nil {
			continue
		}
		qualifiedName := pkg.Path() + "." + obj.Name()
		for _, banned := range c.cfg.BannedFunctions {
			if qualifiedName == banned || obj.Name() == lastSegment(banned) {
				violations = append(violations, Violation{
					Rule:    "banned_function",
					Message: fmt.Sprintf("call to %q is banned for this exercise", qualifiedName),
					Position: token.Position{
						Filename: ident.Name,
					},
				})
			}
		}
	}
	return violations
}

// ── Required function checker (ModeFunctionSignature) ─────────────────────────

// CheckRequiredFunctions verifies that all RequiredFunctions from the config
// are present as exported top-level declarations. It returns one violation per
// missing function.
func (c *Checker) CheckRequiredFunctions(dir string) ([]Violation, error) {
	if len(c.cfg.RequiredFunctions) == 0 {
		return nil, nil
	}
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, dir, nil, parser.AllErrors)
	if err != nil {
		return nil, fmt.Errorf("static analysis: parsing %s: %w", dir, err)
	}

	defined := make(map[string]bool)
	for _, pkg := range pkgs {
		for _, f := range pkg.Files {
			for _, decl := range f.Decls {
				fd, ok := decl.(*ast.FuncDecl)
				if !ok || fd.Name == nil {
					continue
				}
				defined[fd.Name.Name] = true
			}
		}
	}

	var violations []Violation
	for _, spec := range c.cfg.RequiredFunctions {
		if !defined[spec.Name] {
			violations = append(violations, Violation{
				Rule:    "missing_function",
				Message: fmt.Sprintf("required function %q is not defined (expected: %s)", spec.Name, spec.Signature),
			})
		}
	}
	return violations, nil
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func contains(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}

func lastSegment(dotted string) string {
	parts := strings.Split(dotted, ".")
	return parts[len(parts)-1]
}
