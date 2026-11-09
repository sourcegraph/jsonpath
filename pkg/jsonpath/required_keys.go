package jsonpath

import (
	"regexp/syntax"
	"strings"
)

// RequiredKeySubstrings returns strings that the keys of every document from
// which the path selects a node contain, so a search can use them to find
// candidate documents in a text index. Each clause lists alternatives: some
// key of the document contains at least one of them.
//
// Clauses come from the member names the selected nodes are under, and from
// the conditions a node must meet to pass a filter: @property == 'name',
// the longest literal of @property =~ /re/ or search(@property, 're'), and
// member names in queries that must select a node, such as @.a in a test,
// in @.a == 1 or in search(@.a, 're'). Conditions that can be true without
// such a key give no clause, such as negations, @.a != 1, @.a == @.b, names
// of array indexes like @property == '0', and case-insensitive regexes.
func (p *JSONPath) RequiredKeySubstrings() [][]string {
	return segmentsRequiredKeys(p.ast.segments)
}

func segmentsRequiredKeys(segments []*segment) [][]string {
	var clauses [][]string
	for _, seg := range segments {
		switch seg.kind {
		case segmentKindChild:
			clauses = append(clauses, seg.child.requiredKeys()...)
		case segmentKindDescendant:
			clauses = append(clauses, seg.descendant.requiredKeys()...)
		}
	}
	return clauses
}

func (s *innerSegment) requiredKeys() [][]string {
	switch s.kind {
	case segmentDotMemberName:
		return [][]string{{s.dotName}}
	case segmentLongHand:
		// A bracket selects the nodes that any of its selectors select.
		alternatives := make([][][]string, len(s.selectors))
		for i, sel := range s.selectors {
			alternatives[i] = sel.requiredKeys()
		}
		return anyOfRequiredKeys(alternatives)
	}
	return nil
}

func (s *selector) requiredKeys() [][]string {
	switch s.kind {
	case selectorSubKindName:
		return [][]string{{s.name}}
	case selectorSubKindFilter:
		return s.filter.expression.requiredKeys()
	}
	return nil
}

func (e *logicalOrExpr) requiredKeys() [][]string {
	alternatives := make([][][]string, len(e.expressions))
	for i, and := range e.expressions {
		for _, basic := range and.expressions {
			alternatives[i] = append(alternatives[i], basic.requiredKeys()...)
		}
	}
	return anyOfRequiredKeys(alternatives)
}

func (e *basicExpr) requiredKeys() [][]string {
	switch {
	case e.parenExpr != nil && !e.parenExpr.not:
		return e.parenExpr.expr.requiredKeys()
	case e.comparisonExpr != nil:
		return e.comparisonExpr.requiredKeys()
	case e.testExpr != nil && !e.testExpr.not:
		if e.testExpr.filterQuery != nil {
			return e.testExpr.filterQuery.requiredKeys()
		}
		return e.testExpr.functionExpr.requiredKeys()
	}
	return nil
}

// requiredKeys of a comparison. Only == against a literal and the ordering
// operators are false when a query selects no node; == is true when both
// sides select none, and != when one does.
func (e *comparisonExpr) requiredKeys() [][]string {
	switch e.op {
	case regexMatch:
		if e.left.isProperty() {
			return propertyRegexKeys(e.regex.String())
		}
		return e.left.requiredKeys()
	case equalTo, lessThan, lessThanEqualTo, greaterThan, greaterThanEqualTo:
		if e.op == equalTo && e.left.isProperty() && e.right.literal != nil && e.right.literal.string != nil {
			return propertyNameKeys(*e.right.literal.string)
		}
		if e.op == equalTo && e.right.isProperty() && e.left.literal != nil && e.left.literal.string != nil {
			return propertyNameKeys(*e.left.literal.string)
		}
		if e.right.literal != nil {
			return e.left.requiredKeys()
		}
		if e.left.literal != nil {
			return e.right.requiredKeys()
		}
	}
	return nil
}

func (c *comparable) isProperty() bool {
	return c.contextVar != nil && c.contextVar.kind == contextVarProperty
}

func (c *comparable) requiredKeys() [][]string {
	switch {
	case c.singularQuery == nil:
		return nil
	case c.singularQuery.relQuery != nil:
		return segmentsRequiredKeys(c.singularQuery.relQuery.segments)
	case c.singularQuery.absQuery != nil:
		return segmentsRequiredKeys(c.singularQuery.absQuery.segments)
	}
	return nil
}

func (q *filterQuery) requiredKeys() [][]string {
	if q.relQuery != nil {
		return segmentsRequiredKeys(q.relQuery.segments)
	}
	return segmentsRequiredKeys(q.jsonPathQuery.segments)
}

// requiredKeys of search() and match(), which are false unless their first
// argument is a string.
func (e *functionExpr) requiredKeys() [][]string {
	if (e.funcType != functionTypeSearch && e.funcType != functionTypeMatch) || len(e.args) != 2 {
		return nil
	}
	query, re := e.args[0], e.args[1]
	if query.contextVar != nil && query.contextVar.kind == contextVarProperty && re.literal != nil && re.literal.string != nil {
		return propertyRegexKeys(*re.literal.string)
	}
	if query.filterQuery != nil {
		return query.filterQuery.requiredKeys()
	}
	return nil
}

// propertyNameKeys returns a clause for a name that @property equals. Inside
// arrays, @property is the index, which is not a key, so names made only of
// digits have none.
func propertyNameKeys(name string) [][]string {
	if strings.Trim(name, "0123456789") == "" {
		return nil
	}
	return [][]string{{name}}
}

// propertyRegexKeys returns a clause for the longest case-sensitive literal
// that every string matching expr contains.
func propertyRegexKeys(expr string) [][]string {
	re, err := syntax.Parse(expr, syntax.Perl)
	if err != nil {
		return nil
	}
	return propertyNameKeys(requiredLiteral(re.Simplify()))
}

func requiredLiteral(re *syntax.Regexp) string {
	switch re.Op {
	case syntax.OpLiteral:
		if re.Flags&syntax.FoldCase == 0 {
			return string(re.Rune)
		}
	case syntax.OpCapture, syntax.OpPlus:
		return requiredLiteral(re.Sub[0])
	case syntax.OpRepeat:
		if re.Min >= 1 {
			return requiredLiteral(re.Sub[0])
		}
	case syntax.OpConcat:
		longest := ""
		for _, sub := range re.Sub {
			if lit := requiredLiteral(sub); len(lit) > len(longest) {
				longest = lit
			}
		}
		return longest
	}
	return ""
}

// anyOfRequiredKeys returns the clauses of a document that meets at least one
// of the alternatives' clauses: one clause made of the first clause of each.
// If an alternative has no clauses, any document may meet it.
func anyOfRequiredKeys(alternatives [][][]string) [][]string {
	if len(alternatives) == 1 {
		return alternatives[0]
	}
	var clause []string
	for _, clauses := range alternatives {
		if len(clauses) == 0 {
			return nil
		}
		clause = append(clause, clauses[0]...)
	}
	return [][]string{clause}
}
