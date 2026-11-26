package jsonpath

import (
	"encoding/json"
	"testing"

	"github.com/sourcegraph/jsonpath/pkg/jsonpath/config"
	"github.com/stretchr/testify/assert"
	"go.yaml.in/yaml/v4"
)

var semverConfigs = map[string][]config.Option{
	"default": nil,
	// The options Sourcegraph's jsonpath: search filter uses.
	"sourcegraph": {config.WithPropertyNameExtension(), config.WithLazyContextTracking()},
}

// TestSemver checks semver(value, range) for versions and ranges as they are
// written in package.json files, lockfiles and GitHub Actions workflows.
func TestSemver(t *testing.T) {
	tests := []struct {
		value, rng string
		want       bool
	}{
		// Exact versions, with the loose forms that config files use.
		{"1.2.3", "1.2.3", true},
		{"1.2.3", "1.2.4", false},
		{"v1.2.3", "=1.2.3", true},
		{"V1.2.3", "==1.2.3", true},
		{"1.2.3", "<1.10.0", true}, // numeric, not text, order
		{"1.10.0", "<1.9", false},
		{"1.2.3+build.5", "1.2.3", true},
		{"1.2.3", ">1.2.3", false},
		{"1.2.3", ">=1.2.3", true},
		{"1.2.3", "<=1.2.3", true},
		{"1.2.3", "<1.2.3", false},

		// Prereleases sort before their release, by identifier.
		{"2.0.0-rc.1", "<2.0.0", true},
		{"2.0.0-rc.1", ">=2.0.0", false},
		{"2.0.0-rc.10", ">2.0.0-rc.9", true},  // numeric identifiers
		{"2.0.0-beta", "<2.0.0-alpha", false}, // text identifiers
		{"2.0.0-1", "<2.0.0-alpha", true},     // numbers before text
		{"2.0.0-rc.1", "<2.0.0-rc.1.1", true}, // shorter first
		{"3.12.0b1", "<3.12.0", true},         // Python's prerelease without a hyphen
		{"1.0.0rc1", ">=1.0.0-rc0", true},
		// A partial upper bound excludes the prereleases of its first version,
		// as in npm: 2.0.0-rc.1 is a version 2.
		{"2.0.0-rc.1", "<2", false},
		{"2.0.0-rc.1", "2", false},

		// Versions with fewer parts are every version that starts with them.
		{"actions/checkout@v4", ">=4", true},
		{"actions/checkout@v4", "<4", false},
		{"actions/checkout@v3", "<4", true},
		{"actions/checkout@v4", "4.1.7", true},
		{"actions/checkout@v4", "<4.2", true},
		{"actions/checkout@v4", ">=5", false},
		{"actions/setup-node@v4.0.2", "<4.1", true},
		{"18", "18.20.4", true},
		{"18", "19.0.0", false},
		{"16.x", ">=16.13.0", true},
		{"1.21.x", ">=1.22", false},
		{"3.x", "3.12", true},
		{"*", "<1", true},
		{"x", "99.0.0", true},

		// Ranges select dependencies that may resolve to a matching version.
		{"^17.0.2", "<18", true},
		{"^18.2.0", "<18", false},
		{"^18.2.0", "18.3.1", true}, // the range accepts 18.3.1
		{"^18.2.0", "19.0.0", false},
		{"^18", "18.0.0", true},
		{"^0.2.3", "0.2.9", true},
		{"^0.2.3", "0.3.0", false},
		{"^0.0.3", "0.0.4", false},
		{"^0.0", "0.0.9", true},
		{"^0.0", "0.1.0", false},
		{"~1.2.3", "1.2.9", true},
		{"~1.2.3", "1.3.0", false},
		{"~1.2", "1.2.0", true},
		{"~1", "1.9.0", true},
		{"~1", "2.0.0", false},
		{">=14.24.0", ">=20", true},
		{">=14.24.0", "<14", false},
		{">= 16", "16.0.0", true},
		{">=1.2.3 <2.0.0", "2.0.0", false},
		{">=1.2.3 <2.0.0", "1.9.9", true},
		{">=7.4,<8.0", "7.4.33", true},
		{">1.2", "1.2.9", false},
		{">1.2", "1.3.0", true},
		{"<=1.2", "1.2.9", true},
		{"<=1.2", "1.3.0", false},
		{"1.2.3 - 2.3.4", "2.3.4", true},
		{"1.2.3 - 2.3.4", "2.3.5", false},
		{"1.2 - 2.3", "2.3.9", true},
		{"1.2 - 2.3", "1.1.9", false},
		{"20 || 22 || 24", "21.0.0", false},
		{"20 || 22 || 24", "22.11.0", true},
		{"^7.4|^8.0", "8.2.0", true}, // Composer
		{">=2 <1", "1.5.0", false},
		{">=2.17.0 <3.0.0", "^2.19.0", true}, // two ranges overlap
		{">=2.17.0 <3.0.0", "^3.4.0", false},

		// Lockfiles and package managers.
		{"pnpm@9.15.0+sha512.76e2379760a4328ec4415815bcd6628dee727af3779aaa4c914e3944156c4299921a89f976381ee107d41f12cfa4b66681ca9c718f0668fa0831ed4c6d8ba56c", ">=9.12", true},
		{"18.2.0(react@18.2.0)", "<18.3", true}, // pnpm-lock.yaml
		{"react-dom@18.2.0(react@18.2.0)", "18.2.0", true},
		{"npm:react@^18.2.0", "18.3.1", true},
		{"lodash@npm:^4.17.21", "4.17.20", false}, // a Yarn Berry key
		{"workspace:^1.2.0", "<1.2", false},
		{"v5.4.21", "^5.4", true}, // composer.lock

		// Values that aren't versions are false, so their negation is true.
		{"catalog:", "*", false},
		{"workspace:*", "<1", true},
		{"latest", "*", false},
		{"main", "*", false},
		{"lts/*", "*", false},
		{"github:foo/left-pad", "*", false},
		{"git+https://github.com/foo/bar.git#v1.2.3", "*", false},
		{"file:../shared", "*", false},
		{"actions/checkout@8f4b7f84864484a7bf31766abe9204da3cbe65b3", "*", false},
		{"actions/checkout@11bd71901bbe5b1630ceea73d27597364c9af683", "*", false},
		{"docker://alpine:3.18", "*", false},
		{"./.github/actions/setup", "*", false},
		{"node:18-alpine", "*", false},
		{"1.2.3.4", "*", false},
		{"1.2-beta", "*", false},
		{"1.2.3-", "*", false},
		{"1.2.3+", "*", false},
		{"99999999999999999999.0.0", "*", false},
		{"", "*", false},
		{"~> 1.2", "*", false},
		{">=", "*", false},
	}
	for name, opts := range semverConfigs {
		for _, tt := range tests {
			value, _ := json.Marshal(tt.value)
			rng, _ := json.Marshal(tt.rng)
			// The range as a literal, and as a value in the document.
			for _, expr := range []string{
				`$[?semver(@.v, '` + tt.rng + `')]`,
				`$[?semver(@.v, $.x.r)]`,
			} {
				doc := `{"x": {"v": ` + string(value) + `, "r": ` + string(rng) + `}}`
				var node yaml.Node
				assert.NoError(t, yaml.Unmarshal([]byte(doc), &node))
				path, err := NewPath(expr, opts...)
				if !assert.NoError(t, err, expr) {
					continue
				}
				got := len(path.Query(&node)) == 1
				assert.Equal(t, tt.want, got, "%s: semver(%q, %q)", name, tt.value, tt.rng)
			}
		}
	}
}

func TestSemverFilter(t *testing.T) {
	yamlData := `
python-version: 3.10
node-version: 18
enabled: true
dependencies:
  react: "^17.0.2"
  left-pad: "github:foo/left-pad"
  jquery: "catalog:"
  lodash: "4.17.21"
jobs:
  build:
    steps:
      - uses: actions/checkout@v3
      - run: make
      - uses: actions/setup-node@v4
`
	var node yaml.Node
	assert.NoError(t, yaml.Unmarshal([]byte(yamlData), &node))

	tests := []struct {
		path     string
		expected []string
	}{
		{`$.dependencies[?semver(@, '<18')]`, []string{"^17.0.2", "4.17.21"}},
		// Values that aren't versions are excluded by semver() and kept by its negation.
		{`$.dependencies[?!semver(@, '<18')]`, []string{"github:foo/left-pad", "catalog:"}},
		{`$.dependencies[?semver(@, '<18') && @property != 'lodash']`, []string{"^17.0.2"}},
		{`$.jobs.*.steps[?semver(@.uses, '<4')].uses`, []string{"actions/checkout@v3"}},
		// A missing member is false.
		{`$.jobs.*.steps[?!semver(@.uses, '<4')]['run','uses']`, []string{"make", "actions/setup-node@v4"}},
		// Numbers are read as written: YAML's 3.10 is 3.10, not 3.1.
		{`$[?semver(@, '>=3.10 <4')]`, []string{"3.10"}},
		{`$[?semver(@, '18.20.4')]`, []string{"18"}},
		// Booleans, objects and arrays are not versions.
		{`$[?semver(@, '*')]`, []string{"3.10", "18"}},
		{`$..[?semver(@, '*')]`, []string{"3.10", "18", "^17.0.2", "4.17.21", "actions/checkout@v3", "actions/setup-node@v4"}},
	}
	for name, opts := range semverConfigs {
		for _, tt := range tests {
			t.Run(name+"/"+tt.path, func(t *testing.T) {
				path, err := NewPath(tt.path, opts...)
				assert.NoError(t, err)
				var values []string
				for _, r := range path.Query(&node) {
					values = append(values, r.Value)
				}
				assert.Equal(t, tt.expected, values)
			})
		}
	}
}

func TestSemverParse(t *testing.T) {
	path, err := NewPath(`$[?semver(@, '>=1.2')]`)
	assert.NoError(t, err)
	assert.Equal(t, `$[?semver(@, '>=1.2')]`, path.String())

	for expr, wantErr := range map[string]string{
		`$[?semver(@, 'catalog:')]`:  "invalid version or range in semver: catalog:",
		`$[?semver('latest', @)]`:    "invalid version or range in semver: latest",
		`$[?semver(@, 4)]`:           "semver arguments must be strings or queries",
		`$[?semver(@)]`:              "expected ','",
		`$[?semver(@, '1') == true]`: "", // rejected like search(@, '1') == true
		`$[?semver(@, '1', '2')]`:    "expected ')'",
	} {
		_, err := NewPath(expr)
		if assert.Error(t, err, expr) {
			assert.Contains(t, err.Error(), wantErr, expr)
		}
	}

	_, err = NewPath(`$[?semver(@, '1')]`, config.WithStrictRFC9535())
	if assert.Error(t, err) {
		assert.Contains(t, err.Error(), "unknown function: semver")
	}
}
