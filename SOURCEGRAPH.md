# Sourcegraph changes

This repository is a long-term fork of [pb33f/jsonpath](https://github.com/pb33f/jsonpath). It exists for Sourcegraph's experimental `jsonpath:` search filter, which compiles paths with `config.WithPropertyNameExtension()` and `config.WithLazyContextTracking()`. `main` is the fork's only branch; to follow upstream, add it as a remote: `git remote add upstream https://github.com/pb33f/jsonpath`.

This file lists every way `main` differs from upstream v0.8.2. Merging from upstream, upstreaming a fix, or reviewing the fork starts here. Update it in the same commit as the change (see [AGENTS.md](AGENTS.md)).

## Fork setup

**Module path and base.** The module is `github.com/sourcegraph/jsonpath`, so libopenapi can keep depending on upstream without a `replace` directive pulling in our changes. The fork is based on v0.8.2, the last release on `go.yaml.in/yaml/v4`; v0.8.4 moved to `pb33f/go-yaml`, while Sourcegraph and the libopenapi version it uses are on `go.yaml.in/yaml/v4`.

- Files: `go.mod`, every internal import, `README.md` (fork note).

**Web playground.** `web/`, `cmd/wasm`, and the committed `web/src/assets/wasm/lib.wasm` are upstream's playground. The fork does not deploy or rebuild it, so `lib.wasm` predates our changes. `cmd/wasm` still compiles (`GOOS=js GOARCH=wasm go build ./cmd/wasm`).

## Extensions

### Regex match operator `=~`

`<comparable> =~ /re/` and `/re/i` in filter expressions, using [Jayway JSONPath](https://github.com/json-path/JsonPath) syntax. It replaces `search(@, '...')` for users who write the expression inside a quoted search query, where a regex in a JSONPath string needed three levels of escaping.

- Same matching as `search()`: unanchored Go RE2. The left side must be a string; a missing or non-string value is false.
- `\/` is a slash inside the literal; every other escape is passed to RE2 unchanged. `i` is the only flag and becomes a `(?i)` prefix, which `String()` prints.
- An invalid regex, another flag, an unterminated literal, or a non-regex right side such as `@ =~ 'x'` is a parse error.
- `config.WithStrictRFC9535()` rejects it: the tokenizer only scans a regex literal after `=~` in JSONPath Plus mode.
- Code: `token.REGEX` and `Tokenizer.scanRegex` (`pkg/jsonpath/token/token.go`); `parseComparisonExpr` and `regexOperandError` (`pkg/jsonpath/parser.go`); `comparisonExpr.regex` and the `regexMatch` operator (`pkg/jsonpath/filter.go`); the early return in `comparisonExpr.Matches` (`pkg/jsonpath/yaml_query.go`).
- Examples, for `dependencies: {"@workiva/frugal": "^1.0.0", react: "18.2.0"}` and `jobs: {build: {steps: [{uses: actions/checkout@v4}, {run: make}]}}`:
  - `$.dependencies[?@property =~ /^@workiva/]` returns `"^1.0.0"`.
  - `$.dependencies[?@ =~ /^\d+[.]\d+/]` returns `"18.2.0"`.
  - `$.jobs.*.steps[?@.uses =~ /^actions\/checkout@/]` returns the first step.
  - `$.dependencies[?!(@ =~ /^\^/)]` returns `"18.2.0"`; combine with `&&` and `||` like any comparison.
- Tests: `TestRegexMatchOperator` (`pkg/jsonpath/jsonpath_plus_test.go`, run under the default and Sourcegraph options), regex cases in `TestParser` and `TestTokenizer`. Upstream's `TestTokenizer_categorize` cases that expected `=~ /.../` to be illegal now expect it to tokenize.
- Upstream: not proposed.
