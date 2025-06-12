# Agent Guidelines for the IMAP Library

> **Target toolchain:** Go 1.24.x (latest point-release: **1.24.3**, 6 May 2025).
> Set `go 1.24` in `go.mod`; older versions are **not** supported. ([go.dev][1])

---

## 1  Design philosophy

| Principle                                  | Why it matters                                                                                                    |
| ------------------------------------------ | ----------------------------------------------------------------------------------------------------------------- |
| **Single-responsibility agents**           | Easier testing, clearer contracts—no god-objects.                                                                 |
| **Context-first APIs**                     | Cancellation and deadlines propagate without extra params.                                                        |
| **Fail fast, never ignore `error`**        | Wrap or return every error with `%w`; no `//nolint:errcheck`.                                                     |
| **No hidden globals**                      | Only `DefaultDialer`, documented and overridable.                                                                 |
| **Concurrency ≠ contention**               | Prefer per-connection goroutines + channels; if shared state is unavoidable use `sync/atomic` or a small `Mutex`. |
| **Use new language features when helpful** | Generic helpers, `errors.Join`, `maps.Clone`, etc.—but never “cleverness” for its own sake.                       |

---

## 4  Error handling & logging

* Always wrap: `return fmt.Errorf("read greeting: %w", err)`.
* Parallel tasks ➜ collect via `errgroup.Group`, then merge with `errors.Join`.
* Use **one** `log/slog` instance per agent, enriched with `{component: "imap/agent"}`.

---

## 5  Concurrency contracts

1. Reader & writer goroutines share the TCP connection; demux by IMAP tag.
2. A cancelled `context.Context` **must** close the socket and shut down goroutines within 100 ms.
3. Public methods are concurrency-safe **only** when the doc comment says so.

---

## 6  Testing strategy

| Layer                   | Approach                                                                                  |
| ----------------------- | ----------------------------------------------------------------------------------------- |
| Tokenizer / parser      | Table-driven unit tests (≥ 90 % coverage). Include malformed literals & UTF-7 edge cases. |
| Parser entry            | `go test -fuzz` with seeds saved under `testdata/fuzz`.                                   |
| Races / goroutine leaks | CI runs `go test -race` and `go test -run=LeakCheck` with `go.uber.org/goleak`.           |
| Benchmarks              | `go test -bench=.` on `parser` and `selector`; guard allocations.                         |

---

## 7  CI gates (`.github/workflows/ci.yml`)

```yaml
go-version: '^1.24'
steps:
  - run: go vet ./...
  - run: go test ./... -race -coverprofile=coverage.out
  - run: go test -fuzz=Fuzz -fuzztime=30s ./internal/parser
  - run: golangci-lint run --timeout 3m
```

Fail the pipeline on **any** linter warning.

---

## 8  Contribution checklist

* [ ] Public symbols documented, with runnable `Example…` tests.
* [ ] No unchecked errors (`errcheck ./...` is clean).
* [ ] No duplication > 40 tokens (`dupl`).
* [ ] Benchmarks regress ≤ 5 %.
* [ ] `go test -race ./...` passes.

---

## 9  Deprecation & refactors

Breaking API changes follow semver via `/vN` module paths. Internal refactors that improve clarity or adopt new 1.24 features may land at any time.

---

### Appendix A – Go 1.24 features worth using

| Feature                                | Where we’ll use it                                     |
| -------------------------------------- | ------------------------------------------------------ |
| **Generic type aliases**               | `type Flag[T ~string] = T` for compile-time flag sets. |
| **`errors.Join`**                      | Aggregate shutdown errors across goroutines.           |
| **Faster map growth**                  | High-churn caches in `selector`.                       |
| **Dir-scoped `fs.FS`**                 | Future maildir import tooling.                         |
| **Histogram API in `runtime/metrics`** | Per-agent latency / throughput metrics.                |

See the Go 1.24 release notes for full details. ([tip.golang.org][2])

[1]: https://go.dev/doc/devel/release?utm_source=chatgpt.com "Release History - The Go Programming Language"
[2]: https://tip.golang.org/doc/go1.24?utm_source=chatgpt.com "Go 1.24 Release Notes - The Go Programming Language"
