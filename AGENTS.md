# AGENTS.md

## Purpose

This package is the shared CLI framework for GoStartKit.

It is not just a command parser. It is the single source of truth for:

- CLI execution
- REPL execution
- shell completion
- help/usage output
- machine-readable `spec`
- generated docs
- lifecycle hooks, middleware, and observers

Any extension MUST preserve that unified model.

## Core Architecture

The package is built around one command tree and one execution pipeline.

Key flow:

1. `App` builds the effective root command.
2. `Registry` indexes commands, aliases, and built-ins.
3. `Resolver` turns argv or REPL tokens into an `Invocation`.
4. `Dispatcher` runs help, built-ins, or commands with hooks/middleware/observers.

Important files:

- `cmd.go`
  - `App` lifecycle
  - root command synthesis/merge
  - global helpers such as `SetFlags`, `AddCommands`, `Execute`

- `resolver.go`
  - shared CLI/REPL resolution
  - config/env/global flag application
  - positional validation

- `dispatcher.go`
  - invocation dispatch
  - hook/middleware/observer execution

- `registry.go`
  - command path lookup
  - alias handling
  - built-in registration and shadowing rules

- `runtime.go`
  - `CLIRuntime`, `REPLRuntime`, `AutoRuntime`
  - default entrypoint behavior

- `repl.go`, `repl_terminal.go`, `repl_split.go`
  - REPL driver, line execution, history, completion UI

- `completion.go`
  - shell completion and programmatic completion

- `flag.go`, `flag_defs.go`, `flag_runtime.go`-like behavior within current files
  - flag definition, metadata, cached definitions, runtime isolation

- `spec.go`, `docs.go`, `surfaces.go`, `metadata.go`
  - external contract export
  - surface-aware overrides
  - extension metadata cloning

## Non-Negotiable Invariants

### 1. One command tree drives everything

Do NOT add separate models for:

- REPL-only command routing
- docs-only command definitions
- spec-only schemas
- completion-only registries

CLI, REPL, help, completion, `spec`, and `docs` MUST keep sharing the same effective command tree.

### 2. Root compatibility must remain intact

This package supports both:

- legacy `App.Commands` / `App.SetFlags`
- explicit `App.Root`

Do NOT break either mode.

If `App.Root.SubCommands` and `App.Commands` both exist:

- the effective root still merges both
- root-defined subcommand names win
- duplicate names from `App.Commands` are skipped

### 3. Built-in shadowing is intentional

Built-ins such as:

- `help`
- `completion`
- `spec`
- `docs`
- `repl` when enabled

must remain shadowable by user-defined commands of the same name.

Do NOT force built-ins to override user commands.

### 4. CLI and REPL must share parsing semantics

`Run`, `RunLine`, completion, and REPL dispatch MUST continue to resolve through the same command/flag/positional rules where possible.

If parsing changes in one surface, verify the others.

### 5. Runtime state must stay isolated

Flag definitions are cached, but per-invocation mutable state must stay fresh.

Do NOT introduce leaks across:

- repeated CLI runs
- repeated `RunLine` calls
- REPL sessions
- metadata returned from `Lookup`, spec export, or docs generation

Be especially careful with:

- slices
- maps
- cached structs
- extension payloads

### 6. External contract stability matters

`spec`, docs generation, help output, completion behavior, and runtime selection are public behavior.

Treat changes here as API changes even when Go signatures do not change.

## Extension Rules

### Adding a new capability

Prefer extending the shared model instead of adding side channels.

Good examples:

- new flag metadata on `Flag`
- new positional metadata on `PositionalArg`
- new surface-aware overrides
- new built-in that resolves through the existing registry/dispatcher path

Avoid:

- ad hoc globals
- special-case command execution paths
- REPL-only hidden parsing rules

### Adding built-ins

If you add a built-in:

- register it in `registry.go`
- preserve user-command shadowing
- add completion coverage if it takes arguments
- add spec/docs/help expectations if externally visible

### Changing parsing

If you touch parsing or resolution logic, review at minimum:

- `resolver.go`
- `completion.go`
- `repl.go`
- `repl_split.go`
- `cmd_test.go`
- `repl_test.go`
- `architecture_test.go`

Small parser changes often affect:

- `help`
- `--`
- interspersed flags
- inherited/root flags
- completion state

### Changing flag behavior

If you touch flag definitions or metadata:

- preserve cacheability when possible
- preserve runtime isolation
- keep env/config/default precedence intact
- verify `Spec()` and docs still reflect the same metadata

### Changing REPL behavior

Keep these expectations intact:

- command errors do not kill the REPL
- prompt fallback order remains sensible
- history hooks do not silently disappear
- terminal raw-mode handling does not leak into command execution

Changes to REPL UI must also consider completion hint formatting and terminal rendering tests.

## Coding Conventions

- Keep code explicit and Go-idiomatic.
- Prefer minimal diffs over broad rewrites.
- Avoid new dependencies unless clearly required.
- Preserve exported APIs unless the task explicitly requires a breaking change.
- Prefer extending existing types/functions over introducing parallel abstractions.
- Clone mutable metadata when exposing cached or shared definitions.
- Keep comments short and only where behavior is non-obvious.

## Documentation Rules

If behavior changes, update the matching documentation in the same change:

- `README.md`
- `README.zh-CN.md`
- `doc.go` for package-level behavior summaries when relevant

If you change:

- runtime selection
- built-ins
- config precedence
- REPL behavior
- spec/docs export

then docs are part of the implementation, not optional follow-up.

## Testing Expectations

Run the smallest relevant tests while iterating, then run broader checks before finishing.

Useful targeted commands:

- `go test ./...`
- `go test -run 'TestSharedCommandTreeRoutesCLIAndREPL|TestResolverPreservesFlagsPositionalsConfigAndEnv|TestRegistryBuiltinsRemainCompatible'`
- `go test -run 'TestAppRoot.*|TestAppComplete.*|TestAppConfigBindingPrecedence|TestAppMiddlewareOrder|TestAppObserversReceiveLifecycleEvents'`
- `go test -run 'TestREPL.*|TestRunLine.*|TestCompleteLine.*|TestFormatCompletion.*'`
- `go test -run 'TestAppSpec.*|TestSpec.*|TestAppSpecForSurfaceOverrides'`

High-value test files:

- `architecture_test.go`
- `cmd_test.go`
- `runtime_test.go`
- `repl_test.go`
- `flag_runtime_test.go`
- `flag_proto_test.go`
- `spec_surface_test.go`

## Current Baseline Notes

At the time this file was written, `go test ./...` is not fully green because of existing REPL completion rendering failures:

- `TestTerminalSessionCompleteRendersCompletionListFromLineStart`
- `TestFormatCompletionDisplayLine`

Do NOT assume every red test was caused by your change.
If your work is unrelated to REPL terminal rendering, keep these baseline failures separate in your final report unless you intentionally fix them.

## Hard Constraints

- Do NOT split CLI and REPL into separate command-definition systems.
- Do NOT break `DefaultApp` helpers for consumers relying on the global instance.
- Do NOT change precedence order without updating tests and docs together.
- Do NOT mutate cached/shared metadata in place.
- Do NOT refactor unrelated files just because you are nearby.
- Do NOT silently change external contract fields in `Spec()` output.

## Definition of Done

A change in this package is complete only when:

- affected behavior is covered by tests or existing tests were updated intentionally
- documentation is updated when user-visible behavior changed
- CLI, REPL, completion, and spec/docs impacts were considered together
- no unrelated package behavior was changed accidentally
