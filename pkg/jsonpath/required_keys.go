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
// such a key give no clause, such as negations, @.a != 1, @.a == @.b, and
// names of array indexes like @property == '0'.
//
// When ignoreCase is true, keys contain the strings ignoring case, the way a
// regexp with (?i) matches them, and literals of case-insensitive regexes
// such as /sdk/i also give clauses. Otherwise keys contain them exactly.
func (p *JSONPath) RequiredKeySubstrings(ignoreCase bool) [][]string {
	return segmentsRequiredKeys(p.ast.segments, ignoreCase)
}

func segmentsRequiredKeys(segments []*segment, ignoreCase bool) [][]string {
	var clauses [][]string
	for _, seg := range segments {
		switch seg.kind {
		case segmentKindChild:
			clauses = append(clauses, seg.child.requiredKeys(ignoreCase)...)
		case segmentKindDescendant:
			clauses = append(clauses, seg.descendant.requiredKeys(ignoreCase)...)
		}
	}
	return clauses
}

func (s *innerSegment) requiredKeys(ignoreCase bool) [][]string {
	switch s.kind {
	case segmentDotMemberName:
		return [][]string{{s.dotName}}
	case segmentLongHand:
		// A bracket selects the nodes that any of its selectors select.
		alternatives := make([][][]string, len(s.selectors))
		for i, sel := range s.selectors {
			alternatives[i] = sel.requiredKeys(ignoreCase)
		}
		return anyOfRequiredKeys(alternatives)
	}
	return nil
}

func (s *selector) requiredKeys(ignoreCase bool) [][]string {
	switch s.kind {
	case selectorSubKindName:
		return [][]string{{s.name}}
	case selectorSubKindFilter:
		return s.filter.expression.requiredKeys(ignoreCase)
	}
	return nil
}

func (e *logicalOrExpr) requiredKeys(ignoreCase bool) [][]string {
	alternatives := make([][][]string, len(e.expressions))
	for i, and := range e.expressions {
		for _, basic := range and.expressions {
			alternatives[i] = append(alternatives[i], basic.requiredKeys(ignoreCase)...)
		}
	}
	return anyOfRequiredKeys(alternatives)
}

func (e *basicExpr) requiredKeys(ignoreCase bool) [][]string {
	switch {
	case e.parenExpr != nil && !e.parenExpr.not:
		return e.parenExpr.expr.requiredKeys(ignoreCase)
	case e.comparisonExpr != nil:
		return e.comparisonExpr.requiredKeys(ignoreCase)
	case e.testExpr != nil && !e.testExpr.not:
		if e.testExpr.filterQuery != nil {
			return e.testExpr.filterQuery.requiredKeys(ignoreCase)
		}
		return e.testExpr.functionExpr.requiredKeys(ignoreCase)
	}
	return nil
}

// requiredKeys of a comparison. Only == against a literal and the ordering
// operators are false when a query selects no node; == is true when both
// sides select none, and != when one does.
func (e *comparisonExpr) requiredKeys(ignoreCase bool) [][]string {
	switch e.op {
	case regexMatch:
		if e.left.isProperty() {
			return propertyRegexKeys(e.regex.String(), ignoreCase)
		}
		return e.left.requiredKeys(ignoreCase)
	case equalTo, lessThan, lessThanEqualTo, greaterThan, greaterThanEqualTo:
		if e.op == equalTo && e.left.isProperty() && e.right.literal != nil && e.right.literal.string != nil {
			return propertyNameKeys(*e.right.literal.string)
		}
		if e.op == equalTo && e.right.isProperty() && e.left.literal != nil && e.left.literal.string != nil {
			return propertyNameKeys(*e.left.literal.string)
		}
		if e.right.literal != nil {
			return e.left.requiredKeys(ignoreCase)
		}
		if e.left.literal != nil {
			return e.right.requiredKeys(ignoreCase)
		}
	}
	return nil
}

func (c *comparable) isProperty() bool {
	return c.contextVar != nil && c.contextVar.kind == contextVarProperty
}

func (c *comparable) requiredKeys(ignoreCase bool) [][]string {
	switch {
	case c.singularQuery == nil:
		return nil
	case c.singularQuery.relQuery != nil:
		return segmentsRequiredKeys(c.singularQuery.relQuery.segments, ignoreCase)
	case c.singularQuery.absQuery != nil:
		return segmentsRequiredKeys(c.singularQuery.absQuery.segments, ignoreCase)
	}
	return nil
}

func (q *filterQuery) requiredKeys(ignoreCase bool) [][]string {
	if q.relQuery != nil {
		return segmentsRequiredKeys(q.relQuery.segments, ignoreCase)
	}
	return segmentsRequiredKeys(q.jsonPathQuery.segments, ignoreCase)
}

// requiredKeys of search() and match(), which are false unless their first
// argument is a string.
func (e *functionExpr) requiredKeys(ignoreCase bool) [][]string {
	if (e.funcType != functionTypeSearch && e.funcType != functionTypeMatch) || len(e.args) != 2 {
		return nil
	}
	query, re := e.args[0], e.args[1]
	if query.contextVar != nil && query.contextVar.kind == contextVarProperty && re.literal != nil && re.literal.string != nil {
		return propertyRegexKeys(*re.literal.string, ignoreCase)
	}
	if query.filterQuery != nil {
		return query.filterQuery.requiredKeys(ignoreCase)
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

// propertyRegexKeys returns a clause for the longest literal that every
// string matching expr contains. Literals that ignore case, as in /sdk/i,
// count only when ignoreCase is true.
func propertyRegexKeys(expr string, ignoreCase bool) [][]string {
	re, err := syntax.Parse(expr, syntax.Perl)
	if err != nil {
		return nil
	}
	return propertyNameKeys(requiredLiteral(re.Simplify(), ignoreCase))
}

func requiredLiteral(re *syntax.Regexp, ignoreCase bool) string {
	switch re.Op {
	case syntax.OpLiteral:
		if re.Flags&syntax.FoldCase == 0 {
			return string(re.Rune)
		}
		if ignoreCase {
			// The parser stores case-insensitive literals in upper case.
			return strings.ToLower(string(re.Rune))
		}
	case syntax.OpCapture, syntax.OpPlus:
		return requiredLiteral(re.Sub[0], ignoreCase)
	case syntax.OpRepeat:
		if re.Min >= 1 {
			return requiredLiteral(re.Sub[0], ignoreCase)
		}
	case syntax.OpConcat:
		longest := ""
		for _, sub := range re.Sub {
			if lit := requiredLiteral(sub, ignoreCase); len(lit) > len(longest) {
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
