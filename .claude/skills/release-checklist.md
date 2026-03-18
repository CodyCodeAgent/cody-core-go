---
name: release-checklist
description: >
  Run a full consistency checklist after a feature addition or bug fix:
  tests, docs, changelog, examples, README, linting, and commit message.
  Use when the user says things like "检查一下", "准备提交", "checklist",
  "准备推代码", "release check", or after completing a feature/fix.
user_invocable: true
---

# Release Checklist Skill

You are a release-readiness reviewer for the **cody-core-go** open-source project — a Pydantic AI-style agent framework for Go built on cloudwego/eino.

The user has just added a feature or fixed a bug. Your job is to walk through **every consistency checkpoint**, make the necessary changes, and report a final summary. Do NOT skip steps — execute them all.

## Step 0: Understand the Change

1. Run `git diff main --stat` and `git diff main` to understand all changes since main.
2. If there are no changes vs main, run `git diff HEAD~1 --stat` and `git diff HEAD~1` instead.
3. Classify the change:
   - **feat**: new feature / new public API
   - **fix**: bug fix
   - **refactor**: internal restructure, no API change
   - **docs**: documentation only
   - **test**: test only
4. Identify which packages are affected (`agent/`, `output/`, `direct/`, `deps/`, `testutil/`).

## Step 1: Tests

1. Check if new/changed public functions have corresponding test coverage.
2. If tests are missing, write them using `testutil.TestModel` or `testutil.FunctionModel`. Never use real API calls.
3. Use `testutil.Assert*` helpers where applicable.
4. Run `make test-race` — all tests must pass with the race detector.

## Step 2: Exported API Documentation

1. Every new or modified **exported** type, function, or method must have a Go doc comment.
2. Check with: search for `^func [A-Z]` and `^type [A-Z]` in changed files, verify each has a comment above it.
3. Add missing doc comments. Keep them concise — one or two sentences.

## Step 3: CHANGELOG.md

1. Read `CHANGELOG.md`.
2. Add an entry under `## [Unreleased]` in the correct subsection:
   - `### Added` — for new features, new public API
   - `### Fixed` — for bug fixes
   - `### Changed` — for behavioral changes, renames, API modifications
   - `### Removed` — for removed features
   - `### Dependencies` — for dependency updates
3. Write a single-line description. Start with the noun (e.g., "`Agent.Run` now supports..."), not "Added support for...".
4. Do NOT create a new version section — keep it under `[Unreleased]`.

## Step 4: README.md

1. Read `README.md`.
2. If the change introduces a **new public API concept** (new option, new type, new package), add or update the relevant section in README.
3. If the change is a bug fix or internal refactor, skip this step.
4. Keep examples using `testutil.TestModel` so they work without API keys.
5. Maintain consistency with existing README style — code examples use the same import paths and patterns.

## Step 5: Examples

1. If the change adds a **major new capability** (e.g., a new agent option, a new output type, a new package), consider whether it warrants a new example in `examples/`.
2. If modifying existing behavior, check if existing examples in `examples/` still compile: run `go build ./examples/...`
3. All examples must use `testutil.TestModel` — no real API keys.
4. Most bug fixes and minor features do NOT need a new example — use judgment.

## Step 6: CLAUDE.md

1. Read `CLAUDE.md`.
2. If the change affects architecture (new package, new key design pattern, new internal mechanism), update the relevant section.
3. If the change is a bug fix or minor feature, skip this step.

## Step 7: Lint & Format

1. Run `make fmt` to format code.
2. Run `make lint` to check linting.
3. Run `make vet` to check vet.
4. Fix any issues found.

## Step 8: Full Check

1. Run `make check` (vet + lint + test-race). Everything must pass.
2. Run `go mod tidy` and verify no changes: `go mod tidy && git diff --exit-code go.mod go.sum`.

## Step 9: Commit

1. Stage all changed files (be specific — don't use `git add -A`).
2. Use conventional commit format based on the change type:
   - `feat: <description>` — new feature
   - `fix: <description>` — bug fix
   - `refactor: <description>` — refactor
   - `docs: <description>` — docs only
   - `test: <description>` — test only
   - `chore: <description>` — tooling, CI, etc.
3. If the change spans multiple categories, use the primary one and mention others in the body.
4. Ask the user to confirm the commit message before committing.

## Step 10: Final Report

Present a summary table:

```
## Checklist Results

| Step                | Status | Notes                        |
|---------------------|--------|------------------------------|
| Tests               | ...    | ...                          |
| Doc comments        | ...    | ...                          |
| CHANGELOG.md        | ...    | ...                          |
| README.md           | ...    | ...                          |
| Examples            | ...    | ...                          |
| CLAUDE.md           | ...    | ...                          |
| Lint & Format       | ...    | ...                          |
| make check          | ...    | ...                          |
| go mod tidy         | ...    | ...                          |
| Commit              | ...    | ...                          |
```

Status values: DONE, SKIPPED (with reason), FAILED (with action needed).

If everything passed, tell the user: "Ready to push. Run `git push` or I can create a PR for you."
