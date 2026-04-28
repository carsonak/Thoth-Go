# Thoth-Go

A local CLI autograder for learning Go — config-driven, modular, and fast.

## Quick Start

```bash
# Build
go build -o thoth-go ./cmd/thoth-go

# Run
./thoth-go --help
```

## Commands

| Command | Description |
|---------|-------------|
| `start <id>` | Open an exercise in your working directory |
| `check` | Run static + dynamic checks against the current exercise |
| `save` | Snapshot your current progress |
| `load` | Restore a saved snapshot |
| `fetch <topic>` | Download exercises for a topic |
| `reset <id>` | Reset an exercise to its pristine state |
| `progress` | Display overall progress |

## Architecture

```
cmd/thoth-go/       ← main entry point
internal/
  cli/              ← cobra command wiring
  checker/
    static/         ← AST & go/types analysis engine
    dynamic/        ← compilation, test harness
    config.go       ← exercise.yaml struct definitions
  repository/       ← remote fetch, local cache
  state/            ← progress tracking, snapshots
  ui/               ← terminal output helpers (lipgloss)
pkg/
  fsutil/           ← reusable filesystem utilities
```

## Exercise Configuration (`exercise.yaml`)

Each exercise ships an `exercise.yaml` that drives all checker rules — no hardcoded logic.

```yaml
id: "loops-001"
title: "FizzBuzz"
mode: executable          # executable | function_signature | bug_fix
topic: "control-flow"

imports:
  allowed: ["fmt"]
  banned:  []

banned_ast_nodes:
  - "ast.GoStmt"
  - "ast.SelectStmt"

banned_functions: []

test_cases:
  - args: []
    expected_stdout: "1\n2\nFizz\n"
    expected_exit_code: 0
    hidden: false
```
