# Sourcegraph changes

This repository is a long-term fork of [pb33f/jsonpath](https://github.com/pb33f/jsonpath). It exists for Sourcegraph's experimental `jsonpath:` search filter, which compiles paths with `config.WithPropertyNameExtension()` and `config.WithLazyContextTracking()`. `main` is the fork's only branch; to follow upstream, add it as a remote: `git remote add upstream https://github.com/pb33f/jsonpath`.

This file lists every way `main` differs from upstream v0.8.2. Merging from upstream, upstreaming a fix, or reviewing the fork starts here. Update it in the same commit as the change (see [AGENTS.md](AGENTS.md)).

## Fork setup

**Module path and base.** The module is `github.com/sourcegraph/jsonpath`, so libopenapi can keep depending on upstream without a `replace` directive pulling in our changes. The fork is based on v0.8.2, the last release on `go.yaml.in/yaml/v4`; v0.8.4 moved to `pb33f/go-yaml`, while Sourcegraph and the libopenapi version it uses are on `go.yaml.in/yaml/v4`.

- Files: `go.mod`, every internal import, `README.md` (fork note).

**Web playground.** `web/`, `cmd/wasm`, and the committed `web/src/assets/wasm/lib.wasm` are upstream's playground. The fork does not deploy or rebuild it, so `lib.wasm` predates our changes. `cmd/wasm` still compiles (`GOOS=js GOARCH=wasm go build ./cmd/wasm`).
