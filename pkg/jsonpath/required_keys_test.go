package jsonpath

import (
	"strings"
	"testing"

	"github.com/sourcegraph/jsonpath/pkg/jsonpath/config"
	"github.com/stretchr/testify/assert"
	"go.yaml.in/yaml/v4"
)

func TestRequiredKeySubstrings(t *testing.T) {
	tests := map[string]string{
		// Member names the selected nodes are under.
		`$.peerDependencies.react`:                  "peerDependencies,react",
		`$['dependencies','devDependencies'].react`: "dependencies|devDependencies,react",
		`$.jobs.*['runs-on']`:                       "jobs,runs-on",
		`$.a[0].b[*]..c`:                            "a,b,c",
		`$..*`:                                      "",
		`$.a~`:                                      "a",

		// Conditions a node must meet to pass a filter.
		`$..[?@property == 'react']`:                         "react",
		`$..[?'react' == @property && @ != 'catalog:']`:      "react",
		`$.dependencies[?@property == 'jquery'].foo`:         "dependencies,jquery,foo",
		`$[?@.kind == 'Deployment'].spec.replicas`:           "kind,spec,replicas",
		`$.services[?@.privileged == true].image`:            "services,privileged,image",
		`$..[?@.replicas.count > 1 && @['image-tag']]`:       "replicas,count,image-tag",
		`$[?@property == 'name' && $.devEngines]`:            "name,devEngines",
		`$[?@root.engines.node == '20']`:                     "engines,node",
		`$.dependencies[?@property =~ /^@workiva\/(a|b)/]`:   "dependencies,@workiva/",
		`$..with[?search(@property, '^sdk_(channel|v.*)$')]`: "with,sdk_",
		`$.jobs.*.steps[?match(@.uses, '^actions/.*')]`:      "jobs,steps,uses",
		`$..[?@.uses =~ /checkout/]`:                         "uses",
		`$.a[?@.b[?@property == 'c']]`:                       "a,b,c",
		`$..[?(@property == 'react' || @property == 'vue')]`: "react|vue",
		`$..[?@.a && @.b || @property == 'c']`:               "a|c",
		`$['a',?@.b]`:                                        "a|b",
		`$..[?!!@.a]`:                                        "a",

		// Conditions that can be true without the key.
		`$..[?@property == 'react' || @ == 1]`: "",
		`$['a',*]`:                             "",
		`$..[?!(@property == 'react')]`:        "",
		`$..[?!@.a == 1]`:                      "",
		`$..[?!@.a && @.b]`:                    "b",
		`$..[?@property != 'react']`:           "",
		`$..[?@.a != 1]`:                       "",
		`$..[?@.a == @.b]`:                     "",
		`$..[?count(@.a) == 0]`:                "",
		`$..[?length(@.a) > 0]`:                "",
		`$..[?search(@, 'react')]`:             "",
		`$..[?@property =~ /react/i]`:          "",
		`$..[?@property =~ /re(?i:act)/]`:      "re",
		`$..[?@property =~ /react|vue/]`:       "",
		`$..[?@property =~ /\d+/]`:             "",
		`$.steps[?@property == '0']`:           "steps",
		`$..[?@property =~ /^1/]`:              "",
	}
	for expr, want := range tests {
		path, err := NewPath(expr, config.WithPropertyNameExtension(), config.WithLazyContextTracking())
		assert.NoError(t, err, expr)
		var clauses []string
		for _, clause := range path.RequiredKeySubstrings(false) {
			clauses = append(clauses, strings.Join(clause, "|"))
		}
		assert.Equal(t, want, strings.Join(clauses, ","), expr)
	}

	// Ignoring case adds the literals of case-insensitive regexes.
	for expr, want := range map[string]string{
		`$..[?@property =~ /react/i]`:            "react",
		`$..with[?search(@property, '(?i)sdk')]`: "with,sdk",
		`$..[?@property =~ /re(?i:act)/]`:        "act",
		`$..[?@property =~ /^1/i]`:               "",
		`$..[?@property == 'React']`:             "React",
	} {
		path, err := NewPath(expr, config.WithPropertyNameExtension(), config.WithLazyContextTracking())
		assert.NoError(t, err, expr)
		var clauses []string
		for _, clause := range path.RequiredKeySubstrings(true) {
			clauses = append(clauses, strings.Join(clause, "|"))
		}
		assert.Equal(t, want, strings.Join(clauses, ","), expr)
	}
}

// TestRequiredKeySubstringsHoldWhereSelected checks that every document from
// which a path selects a node meets its clauses.
func TestRequiredKeySubstringsHoldWhereSelected(t *testing.T) {
	docs := []string{
		`{"dependencies": {"react": "^18", "vue": "3"}, "devDependencies": {"left-pad": "1"}}`,
		"jobs:\n  build:\n    steps:\n      - uses: actions/checkout@v4\n        with: {sdk: 3.4}\n      - run: make\n",
		`["react", "vue"]`,
		"react: true\nvue: [x]\n",
		"steps: [{run: make}]",
		`{"SDK_Channel": "stable", "React": "18"}`,
	}
	exprs := []string{
		`$..[?@property == 'react' || @property == 'vue']`,
		`$..[?@property == '1' || @property == '0']`,
		`$..[?@property =~ /^1$/]`,
		`$..[?@property =~ /^[a-z]+(Dependencies)$/]`,
		`$.jobs[?@.steps[?@.with.sdk > 3]]`,
		`$..[?@.with || @property == 'react']`,
		`$..steps[?!@.uses]`,
		`$..steps[?@.uses != 'x']`,
		`$..[?@ == 'react']`,
		`$..[?@property =~ /sdk/i]`,
		`$..[?search(@property, '(?i)^react$') || @property == 'vue']`,
	}
	configs := map[string][]config.Option{
		"default":     nil,
		"sourcegraph": {config.WithPropertyNameExtension(), config.WithLazyContextTracking()},
	}
	for name, opts := range configs {
		for _, expr := range exprs {
			path, err := NewPath(expr, opts...)
			assert.NoError(t, err, expr)
			for _, doc := range docs {
				var root yaml.Node
				assert.NoError(t, yaml.Unmarshal([]byte(doc), &root))
				if len(path.Query(&root)) == 0 {
					continue
				}
				keys := strings.Join(mappingKeys(&root), "\n")
				for _, ignoreCase := range []bool{false, true} {
					for _, clause := range path.RequiredKeySubstrings(ignoreCase) {
						met := false
						for _, s := range clause {
							met = met || strings.Contains(keys, s) || ignoreCase && strings.Contains(strings.ToLower(keys), strings.ToLower(s))
						}
						assert.True(t, met, "%s/%s (ignoreCase %v): no key of %q contains one of %q", name, expr, ignoreCase, doc, clause)
					}
				}
			}
		}
	}
}

func mappingKeys(node *yaml.Node) []string {
	var keys []string
	for i, child := range node.Content {
		if node.Kind == yaml.MappingNode && i%2 == 0 {
			keys = append(keys, child.Value)
		}
		keys = append(keys, mappingKeys(child)...)
	}
	return keys
}
