package jsonpath_test

import (
    "github.com/sourcegraph/jsonpath/pkg/jsonpath"
    "github.com/sourcegraph/jsonpath/pkg/jsonpath/config"
    "github.com/stretchr/testify/require"
    "go.yaml.in/yaml/v4"
    "testing"
)

func TestParser(t *testing.T) {
    tests := []struct {
        name    string
        input   string
        invalid bool
    }{
        {
            name:  "Root node",
            input: "$",
        },
        {
            name:  "Single Dot child",
            input: "$.store",
        },
        {
            name:  "Single Bracket child",
            input: "$['store']",
        },
        {
            name:  "Bracket child",
            input: "$['store']['book']",
        },
        {
            name:  "Array index",
            input: "$[0]",
        },
        {
            name:  "Array slice",
            input: "$[1:3]",
        },
        {
            name:  "Array slice with step",
            input: "$[0:5:2]",
        },
        {
            name:  "Array slice with negative step",
            input: "$[5:1:-2]",
        },
        {
            name:  "Filter expression",
            input: "$[?(@.price < 10)]",
        },
        {
            name:  "Nested filter expression",
            input: "$[?(@.price < 10 && @.category == 'fiction')]",
        },
        {
            name:  "Function call",
            input: "$.books[?(length(@) > 100)]",
        },
        {
            name:    "Invalid missing closing ]",
            input:   "$.paths.['/pet'",
            invalid: true,
        },
        {
            name:    "Invalid extra input",
            input:   "$.paths.['/pet')",
            invalid: true,
        },
        {
            name:  "Valid filter",
            input: "$.paths[?(1 == 1)]",
        },
        {
            name:    "Invalid filter",
            input:   "$.paths[?(true]",
            invalid: true,
        },
        {
            name:  "Regex match",
            input: `$.jobs.*.steps[?@.uses =~ /^actions\/checkout@/]`,
        },
        {
            name:  "Regex match combined",
            input: `$[?@.c =~ /(?i)c/ || @property =~ /^a/ && !(@ =~ /b/)]`,
        },
        {
            name:    "Regex match invalid regex",
            input:   `$[?@ =~ /(/]`,
            invalid: true,
        },
        {
            name:    "Regex match unsupported flag",
            input:   `$[?@ =~ /a/g]`,
            invalid: true,
        },
        {
            name:    "Regex match unterminated",
            input:   `$[?@ =~ /a]`,
            invalid: true,
        },
        {
            name:    "Regex match with string operand",
            input:   `$[?@ =~ 'a']`,
            invalid: true,
        },
        {
            name:  "Negated function before &&",
            input: `$[?!search(@.a, 'x') && @.b]`,
        },
        {
            name:  "Negated paren before ||",
            input: `$[?!(@.a == 1) || @.b]`,
        },
        {
            name:  "Negated existence before &&",
            input: `$[?!@.a && @.b]`,
        },
        {
            name:    "Regex literal without =~",
            input:   `$[?@ == /a/]`,
            invalid: true,
        },
    }

    for _, test := range tests {
        t.Run(test.name, func(t *testing.T) {
            path, err := jsonpath.NewPath(test.input)
            if test.invalid {
                require.Error(t, err)
                return
            }
            require.NoError(t, err)
            require.Equal(t, test.input, path.String())
        })
    }
}

func TestParserPropertyNameExtension(t *testing.T) {
    tests := []struct {
        name    string
        input   string
        enabled bool
        valid   bool
    }{
        {
            name:    "Simple property name disabled",
            input:   "$.store~",
            enabled: false,
            valid:   false,
        },
        {
            name:    "Simple property name enabled",
            input:   "$.store~",
            enabled: true,
            valid:   true,
        },
        {
            name:    "Property name in filter disabled",
            input:   "$[?(@~)]",
            enabled: false,
            valid:   false,
        },
        {
            name:    "Property name in filter enabled",
            input:   "$[?(@~)]",
            enabled: true,
            valid:   true,
        },
        {
            name:    "Property name with bracket notation enabled",
            input:   "$['store']~",
            enabled: true,
            valid:   true,
        },
        {
            name:    "Property name with bracket notation disabled",
            input:   "$['store']~",
            enabled: false,
            valid:   false,
        },
        {
            name:    "Chained property names enabled",
            input:   "$.store~.name~",
            enabled: true,
            valid:   true,
        },
        {
            name:    "Property name in complex filter enabled",
            input:   "$[?(@~ && @.price < 10)]",
            enabled: true,
            valid:   true,
        },
        {
            name:    "Property name in complex filter disabled",
            input:   "$[?(@~ && @.price < 10)]",
            enabled: false,
            valid:   false,
        },
        {
            name:    "Missing closing a filter expression shouldn't crash",
            input:   "$.paths.*.*[?(!@.servers)",
            enabled: false,
            valid:   false,
        },
        {
            name:    "Missing closing a filter expression shouldn't crash",
            input:   "$.paths.*.*[?(!@.servers)",
            enabled: false,
            valid:   false,
        },
        {
            name:    "Missing closing a array crash",
            input:   "$.paths.*[?@[\"x-my-ignore\"]",
            enabled: false,
            valid:   false,
        },
    }

    for _, test := range tests {
        t.Run(test.name, func(t *testing.T) {
            var opts []config.Option
            if test.enabled {
                opts = append(opts, config.WithPropertyNameExtension())
            }

            path, err := jsonpath.NewPath(test.input, opts...)
            if !test.valid {
                require.Error(t, err)
                return
            }
            require.NoError(t, err)
            require.Equal(t, test.input, path.String())
        })
    }
}

// TestLogicalNotPrecedence checks that a leading ! negates only the next
// basic expression, as in RFC 9535's grammar, and not the rest of the && / ||
// chain after it.
func TestLogicalNotPrecedence(t *testing.T) {
    var node yaml.Node
    require.NoError(t, yaml.Unmarshal([]byte(`
steps:
  - {id: checkout, name: Checkout, uses: actions/checkout@v4}
  - {id: setup, uses: pnpm/action-setup@v4, with: {version: 9}}
  - {id: pnpm-install, name: Install, run: pnpm i, with: {frozen: true}}
  - {id: check, name: Check licenses, run: ./check}
  - {id: cache, uses: actions/cache@v4, with: {path: x}}
`), &node))

    tests := []struct {
        path     string
        expected []string
    }{
        {`$.steps[?!search(@.id, '^pnpm') && @.name].id`, []string{"checkout", "check"}},
        {`$.steps[?!search(@.id, 'c') || @.with].id`, []string{"setup", "pnpm-install", "cache"}},
        {`$.steps[?!(@.id == 'pnpm-install') && @.with].id`, []string{"setup", "cache"}},
        {`$.steps[?!(@.id == 'checkout') || @.with].id`, []string{"setup", "pnpm-install", "check", "cache"}},
        {`$.steps[?!@.uses && @.name].id`, []string{"pnpm-install", "check"}},
        {`$.steps[?!(@.id =~ /^c/) && @.with].id`, []string{"setup", "pnpm-install"}},
        {`$.steps[?@.with && !(@.id =~ /^c/) || !@.with && @.id == 'check'].id`, []string{"setup", "pnpm-install", "check"}},
        // Lenient beyond RFC 9535: a negated comparison without parentheses.
        {`$.steps[?!@.id == 'checkout' && @.name].id`, []string{"pnpm-install", "check"}},
    }
    for _, tt := range tests {
        t.Run(tt.path, func(t *testing.T) {
            path, err := jsonpath.NewPath(tt.path)
            require.NoError(t, err)
            var ids []string
            for _, r := range path.Query(&node) {
                ids = append(ids, r.Value)
            }
            require.Equal(t, tt.expected, ids)
        })
    }
}
