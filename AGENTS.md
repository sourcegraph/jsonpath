# Sourcegraph jsonpath

This is Sourcegraph's long-term fork of pb33f/jsonpath, published as `github.com/sourcegraph/jsonpath`. `main` is the fork's only branch; upstream is https://github.com/pb33f/jsonpath, which you can add as a remote.

## Record every divergence from upstream

Every change that makes `main` differ from upstream must update [SOURCEGRAPH.md](SOURCEGRAPH.md) in the same commit. This covers new syntax or behaviour, bug fixes, removed features, and build or CI changes. Say what changed, why, which functions it touches, and which tests cover it. Update or delete the entry when the change is reworked, reverted, or merged upstream.

## Conventions

- New syntax is a JSONPath Plus extension: gate it on `config.JSONPathPlusEnabled()` so `config.WithStrictRFC9535()` rejects it. Document user-facing syntax, with examples, in its SOURCEGRAPH.md entry. Keep all Sourcegraph documentation in SOURCEGRAPH.md: README.md stays upstream's apart from the fork note at the top, so it doesn't conflict with upstream changes.
- Keep RFC 9535 behaviour compliant. The compliance suite is a git submodule: run `git submodule update --init`, then `go test ./...`; the root package runs `jsonpath-compliance-test-suite/cts.json`.
- Sourcegraph's `jsonpath:` search filter compiles paths with `config.WithPropertyNameExtension()` and `config.WithLazyContextTracking()`. Test new behaviour under those options as well as the defaults.
- Keep the diff against upstream small so merges stay easy. Add to existing functions rather than restructuring them, and match each file's existing indentation: many files are not gofmt-formatted, and reformatting them would conflict with every upstream change.
