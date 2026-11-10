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

### Key substrings for finding candidate documents

`(*JSONPath).RequiredKeySubstrings(ignoreCase)` returns clauses that every document the path selects a node from meets: some key of the document contains one of each clause's strings, exactly or, with `ignoreCase`, ignoring case. Sourcegraph's `jsonpath:` filter searches its index for files that contain them before it reads and parses each file. The analysis lives here, next to the evaluator, because whether a condition implies a key depends on evaluation details.

- Clauses come from the member names on the path, `@property == 'name'`, the longest literal of `@property =~ /re/`, `search(@property, 're')` and `match(@property, 're')` (literals that ignore case, as in `/sdk/i`, only with `ignoreCase`, lowercased), and the member names of queries that must select a node: tests such as `@.a`, comparisons with a literal (`==`, `<`, `<=`, `>`, `>=`), `@.a =~ /re/`, and the first argument of `search()` and `match()`. `&&` adds clauses; `||` and bracket unions merge the first clause of each alternative into one.
- No clauses come from negations, `!=` (true when the member is missing), comparisons of two queries (true when both are missing), or `count()`, `length()` and `value()`. `@property` is an array element's index, so names made only of digits give none either.
- Code: `pkg/jsonpath/required_keys.go`.
- Tests: `TestRequiredKeySubstrings`, and `TestRequiredKeySubstringsHoldWhereSelected`, which checks each clause against the keys of documents the path selects from, with and without `ignoreCase`, under the default and Sourcegraph options (`pkg/jsonpath/required_keys_test.go`).
- Upstream: not proposed.

## Bug fixes

### A leading `!` binds only to the next basic expression

Upstream parsed everything after `!` as one `||` expression, so `!search(@.uses, 'actions/') && @.name` meant `!(search(...) && @.name)`. RFC 9535's grammar applies `!` only to the following paren expression or test expression, so it now means `(!search(...)) && @.name`. The same applies to `!(...)` and `!@.x`.

- A negated comparison without parentheses, such as `!@.a == 1`, is invalid in RFC 9535. It is still accepted, as upstream did, and now means `!(@.a == 1)` without swallowing what follows.
- Code: the `token.NOT` case of `parseBasicExpr` (`pkg/jsonpath/parser.go`).
- Tests: `TestLogicalNotPrecedence` and the negation cases in `TestParser` (`pkg/jsonpath/parser_test.go`). The compliance suite has no case that combines `!` with `&&` or `||`.
- Upstream: not proposed.

### `search()` and `match()` are false for a missing or non-string value

Upstream returned Nothing instead of false when the first argument selected no node, and `testExpr.Matches` treated any function result other than false or null as true. So `steps[?search(@.uses, '^actions/checkout@')]` also selected every step without `uses`, and `!search(...)` excluded them. RFC 9535 sections 2.4.6 and 2.4.7 say both functions return LogicalFalse unless both arguments are strings, including when the argument is Nothing.

- A function used as a test expression now matches only when it returns true. Today only `match()`, `search()` and the type-selector functions can be used there; the parser rejects `length()`, `count()` and `value()`, and the type selectors already returned false for Nothing.
- Code: `functionExpr.match`, `functionExpr.search` and `isStringArg` (`pkg/jsonpath/yaml_eval.go`); `testExpr.Matches` (`pkg/jsonpath/yaml_query.go`).
- Tests: `TestSearchMatchMissingMember` (`pkg/jsonpath/yaml_query_test.go`, run under the default and Sourcegraph options).
- Upstream: not proposed. Still present in v0.8.5.

### Queries inside a filter no longer change its context variables

Selectors record the current node's `@property`, `@path`, `@parent`, `@parentProperty` and `@index` in one shared filter context as they descend. A relative query, a singular absolute query in a comparison, or a nested filter in one condition therefore overwrote what later conditions saw: `$.jobs[?@['runs-on'] && @property == 'check']` matched nothing, while `... && @property == 'runs-on'` matched. Relative and absolute queries now save that state and restore it when they return.

- Absolute queries used as existence tests (`?$.x`) were not affected; they already run with a fresh context.
- The restore copies a small struct and truncates the path slice; it does not allocate.
- Code: `relQuery.Query` and `absQuery.Query` (`pkg/jsonpath/yaml_query.go`); `filterContextState`, `filterContext.saveState` and `filterContext.restoreState` (`pkg/jsonpath/filter_context.go`).
- Tests: `TestFilterContextIsolation` (`pkg/jsonpath/jsonpath_plus_test.go`, run under the default and Sourcegraph options).
- Upstream: not proposed. Still present in v0.8.5.

### `@parentProperty` and `@path` after a wildcard, union or filter

Upstream evaluates a query one segment at a time over the whole nodelist, but a filter read its parent's name from shared state, which by then held the last child key of the previous filter. With `jobs: {build: {steps: [a, b]}, test: {steps: [c]}}`, `$.jobs.*.steps[?@parentProperty == 'steps']` returned only `a` and `b`. Likewise, a filter's matches did not record their path segment, so `$.jobs[?@property == 'check'].steps[?@path == "$['jobs']['check']['steps'][1]"]` built the path without `['check']` and matched nothing.

- A filter now takes `@parentProperty` from the filtered node's own key, which name, wildcard and filter selectors already record (the index behind `~`), or from the pending array index left by an index, wildcard, slice or filter selector. Filter matches record a pending path segment, as wildcard results do.
- Queries without filters do no extra work; `BenchmarkLazyContextTrackingStress` allocations are unchanged in eager and lazy modes.
- Code: the `selectorSubKindFilter` and `selectorSubKindArrayIndex` cases of `selector.Query` (`pkg/jsonpath/yaml_query.go`).
- Tests: the nodelist cases in `TestFilterContextIsolation` (`pkg/jsonpath/jsonpath_plus_test.go`, run under the default and Sourcegraph options).
- Upstream: not proposed. Present in v0.8.5 by code inspection.
