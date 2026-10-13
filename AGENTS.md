# dbo-deployment-wizard — agent guide

Interactive installer for the castai-dbo Helm chart: collects every
deployment parameter (kubectl context, namespace, chart version,
components, database credentials), shows a review summary, and runs
`helm upgrade --install`.

## Layout

- `internal/api` — the contract between "backend" and "frontend"
- `internal/tui` — UI section of the app. Should only depend on `api` and never
  reference `backend` directly (enforced by a depguard rule in `.golangci.yml`)
- the rest — backend (`internal/backend`; generated mocks live in `mocks/`)

### `api` and `backend` are UI-agnostic

Together they are the wizard's headless logic: everything a frontend
needs, with no knowledge of which frontend exists.

- The implementation makes no assumptions about the UI that drives it:
  no ordering tailored to screens, no return shapes chosen for how one
  view renders them, no behaviour that only makes sense for a TUI.
- Comments are phrased in domain terms. Never mention screens, forms,
  spinners, key presses, "the user picks", or what a frontend does with
  a result. Self-explanatory contract methods get no comment; a comment
  is warranted only to state something specific about the backend's
  behaviour (ordering, failure semantics, caching).
- Anything that is really about the UI belongs in `internal/tui`, next
  to the code that needs it.

## Comments

- Add comments sparingly — only where completely necessary to explain
  underlying logic the code itself can't (workarounds, gotchas,
  non-obvious decisions). If the code is obvious, no comment.
- Keep them short: one or two lines, never multi-paragraph essays.
  Explain the *why*, never restate the *what*.

## Slice conversions

- Use `lo.Map` / `lo.FilterMap` whenever converting one slice into
  another — never a manual `make` + `for` + `append` loop.

## Testing

- Table tests: declare the cases as `map[string]struct{...}` where the map
  key is the test name — never a slice of structs with a `name` field.
  The key doubles as the `t.Run` subtest name, so adding a case
  is a single literal with no name/description drift.

  ```go
  tests := map[string]struct {
      version string
      latest  string
      want    string
  }{
      "matching latest": {version: "1.4.2", latest: "1.4.2", want: "1.4.2 (latest)"},
      "older version":   {version: "1.4.1", latest: "1.4.2", want: "1.4.1"},
  }
  for name, tt := range tests {
      t.Run(name, func(t *testing.T) { ... })
  }
  ```

- Test helpers take `testing.TB`, never `*testing.T`, so benchmarks
  and other test variants can share them.

- Assertions: construct an asserter with `r := require.New(t)` at the
  top of each test, subtest and helper, and call `r.Equal(...)` etc. —
  never the package-level `require.Equal(t, ...)` form.

- Mocks are generated with mockery from `.mockery.yaml` and live in
  `mocks/`; always regenerate with `make gen`, never edit them by hand.
- Run tests with `make test`; lint with `make lint` (or `make fix` to
  auto-fix).
- Work is not done until both `make test` and `make lint` pass.

## Commits

- Never add agent `Co-Authored-By` trailers (or any other agent
  attribution) to commit messages or pull request descriptions.
- Never commit directly to `main` — always work on a branch.
- Never push, force-push, or rewrite already-pushed history unless
  explicitly asked.

## Scope

- Work strictly within this repository — never read or modify files
  in other repositories.
