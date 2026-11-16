# `template/gotext/` -- vendored `text/template` with context cancellation

This directory is a **verbatim copy of Go's standard-library `text/template` package**, plus two
small patches: one adds context-aware execution (`ExecuteContext`), the other a hook that vets values
before they are printed (`PrintCheck`). Both exist to stop **user-supplied** message templates
(`Template: yes`, see the [templating docs](https://ntfy.sh/docs/publish/#message-templating)) from
burning CPU or memory.

- **Source:** Go stdlib `text/template` (+ `internal/fmtsort`), `$(go env GOROOT)/src`
- **Version:** pinned in the repo-root [`.go-version`](../../.go-version); recorded in `GENERATED_FROM`
- **Local modifications:** `patches/` (see [The patch](#the-patch))
- **Update mechanism:** Manual -- `make update-template`, then commit (never autorolled; see [Updating](#updating-when-bumping-the-go-toolchain))

## Why this exists

ntfy lets users send a Go template that is rendered against a JSON body. Go's `text/template`
**cannot be interrupted mid-execution** -- there is no context, no deadline, no cancellation
([golang/go#31107](https://github.com/golang/go/issues/31107) proposed `ExecuteContext` but was
declined, over a bundled context-*values* feature, not cancellation itself). We proposed just the
cancellation half in [golang/go#80380](https://github.com/golang/go/issues/80380) (July 2026); it was
closed the same day as out of scope: `text/template` assumes template authors are trusted, and the
Go team does not treat denial of service through a template as a security issue (see
[golang/go#79014](https://github.com/golang/go/issues/79014)). So a crafted template
with a tight or nested `{{range}}` (e.g. ranging over a large JSON array with a big loop body that
writes no output) can run for tens of seconds on a single request. That is a CPU denial of service
(GHSA-rhwf-xgc9-m9fp).

There is no way to add an interrupt from the outside -- the executor's per-node `walk` loop is
unexported. The only robust fix is to patch the executor itself. Rather than reach for fragile
heuristics (guessing iteration counts, wrapping every function, etc.), we vendor the package and add
the cancellation half of #31107 as a patch: `ExecuteContext(ctx, ...)` that aborts with `ctx.Err()`
when `ctx` is canceled or its deadline passes. The check is a **poll inside `walk` and before every
function call in `evalCall`** of an atomic flag that a `context.AfterFunc` watcher flips -- so it
bounds CPU for *any* template shape (cheap loops, long pipelines, deeply nested calls, and expensive
functions alike), it is exact (observed within one node or call), and it adds no measurable
overhead. If cancellation ever lands upstream after all (unlikely, see #80380), this fork can be
deleted and the call site keeps compiling unchanged.

The one user-facing execution site (`server/server_template.go` `renderTemplate`) wraps execution in
`context.WithTimeout` and calls `ExecuteContext`, mapping `context.DeadlineExceeded` to a `400`.
Trusted templates (operator config: Twilio, `cmd/serve.go`) keep using the standard library -- they
are not user-supplied.

## What's here

| File | Origin |
|------|--------|
| `*.go` (`exec.go`, `funcs.go`, `template.go`, `option.go`, `helper.go`, `doc.go`) | verbatim from `$(go env GOROOT)/src/text/template/`, enumerated with `go list` so files added/removed upstream are picked up automatically |
| `fmtsort/sort.go` | verbatim from `$(go env GOROOT)/src/internal/fmtsort/` -- `exec.go` needs it, and `internal/...` packages can't be imported from outside GOROOT, so it comes along |
| `patches/0001-exec-context.patch` | context cancellation (see below) |
| `patches/0002-print-check.patch` | print hook (see below) |
| `patches_test.go` | ours, not upstream: tests for the patches; `make update-template` keeps it and `make template-check` ignores it (upstream's tests are not vendored, and none is named like this) |
| `GENERATED_FROM` | the exact Go version `make update-template` last regenerated this copy from; provenance, written by that target |

The Go toolchain version this copy is pinned to lives in the repo-root [`.go-version`](../../.go-version)
file (the single source of truth, also consumed by CI and the `make` targets below). `GENERATED_FROM`
must equal it -- `make check` fails otherwise (see below).

We do **not** vendor `text/template/parse` -- it's a normal importable stdlib package and stays a
plain import.

## The patch

`patches/` is a quilt-style ordered series (apply `0001-*`, then `0002-*`, ...). Today there are two.

`0001-exec-context.patch` -- small, and touching only `exec.go`:

- adds `ctx context.Context` and a shared `cancelled *atomic.Bool` to the executor `state`
- adds `ExecuteContext` / `ExecuteTemplateContext`; `Execute` / `ExecuteTemplate` become
  `context.Background()` wrappers, so their behavior and cost are unchanged
- when `ctx.Done() != nil`, arms one `context.AfterFunc` watcher that flips the flag; a
  `checkCancelled` helper polls it and aborts via a `cancelError` that `errRecover` strips to the bare
  `ctx.Err()` (`errors.Is(err, context.DeadlineExceeded)`)
- `checkCancelled` runs in `walk` (per node), and in `evalCall` right before each function/method
  invocation (after its arguments are evaluated, so nested calls are checked as they unwind, not only
  on the way down). A single action node (`{{0 | f | f | ...}}` or `{{f (f (f ...))}}`) is one `walk`
  step, yet its pipeline and nested calls can run arbitrarily long; every pipeline command that does
  work is a call, so this bounds long pipelines too

The flag is a `*atomic.Bool` (not a value) because `walkTemplate` copies `state` for nested
`{{template}}` invocations; a shared pointer keeps one flag across all copies and avoids `go vet`
copylocks. `template.go` is unchanged -- the context is per-call, not stored on the `Template`.

`0002-print-check.patch` -- purely additive, touching `option.go` and `exec.go`:

- adds `PrintCheck(func(...any) error)`, stored in the template's `option` like `missingkey`
- `printValue` calls it right before `fmt.Fprint`, and aborts execution if it returns an error

The executor prints an action's value (`{{$x}}`) with `fmt` directly, and `fmt` has no cycle
detection and expands shared values, so a template could otherwise crash the server or allocate
without bound by printing a value that references itself or nests shared lists. `printValue` is the
single place every action's output goes through, so one check there covers every template shape.
`renderTemplate` passes `template.CheckPrintable` (`template/printable.go`).

Two *mechanical* transforms are applied by `make update-template` with `sed`, **not** the patch --
renaming the package to `gotext`, and rewriting the `internal/fmtsort` import to
`heckel.io/ntfy/v2/template/gotext/fmtsort`. Keeping them out of the patches means they apply to
whatever files `go list` returns, so they survive upstream files being added or removed. (These two
transforms are also the only difference between our 0001 patch and the upstream `text/template` diff.)

Keeping the patches tiny (a few hunks on stable files) is deliberate: it makes re-basing
onto a new Go release cheap.

## Updating (when bumping the Go toolchain)

The copy is **pinned to the Go version in the root `.go-version`**, so it's not frozen -- re-syncing
on a Go bump pulls in all upstream fixes for free. `.go-version` is authoritative and hand-edited; to
bump the toolchain: edit `.go-version`, install that toolchain
(`go install golang.org/dl/<version>@latest && <version> download`), then re-sync:

```
make update-template   # copies the files from your GOROOT and re-applies patches/*.patch
```

`make update-template` **errors** unless your local Go matches `.go-version` -- it validates against
the pin, it never writes it. If the patch hunks no longer apply against the new release, refresh the
patches as part of the bump (regenerate each with `diff -u` against the tree with the previous
patches applied).

`make template-check` (wired into `make check`) has two layers:

1. **Marker check (ungated, runs on any toolchain):** fails if `GENERATED_FROM` != `.go-version`, i.e.
   someone bumped the pin but forgot `make update-template` (or vice versa). This catches the common
   mistake locally, on any developer's Go.
2. **Content check (gated to the pinned Go):** re-derives the copy from `GOROOT + patches` and diffs it
   against what's committed, catching hand-edits and patch problems. It no-ops on a non-pinned
   toolchain so it never fails spuriously.

CI installs exactly `.go-version` (`go-version-file`), so both layers run there. `make release`
additionally refuses to run off the pinned Go, so the content check is never skipped for a release.

## License

These files are copyright The Go Authors, under the BSD-3-Clause license (headers preserved in each
file). That is compatible with ntfy's Apache-2.0 / GPLv2 licensing.
