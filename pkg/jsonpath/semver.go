package jsonpath

import (
	"cmp"
	"strconv"
	"strings"
)

// This file implements the semver() function: a best-effort comparison of the
// loose versions and npm-style ranges found in package.json files, lockfiles
// and GitHub Actions workflows. See SOURCEGRAPH.md for the accepted syntax.

// version is a version with three numeric parts and an optional prerelease.
// Build metadata is dropped when parsing, since it does not affect order.
type version struct {
	nums [3]uint64
	pre  []string // nil for a release
}

// compareVersions orders versions as semver does: by their numeric parts,
// then a prerelease before its release, then by prerelease identifiers.
func compareVersions(a, b version) int {
	for i := range a.nums {
		if c := cmp.Compare(a.nums[i], b.nums[i]); c != 0 {
			return c
		}
	}
	switch {
	case a.pre == nil && b.pre == nil:
		return 0
	case a.pre == nil:
		return 1
	case b.pre == nil:
		return -1
	}
	for i := 0; i < len(a.pre) && i < len(b.pre); i++ {
		an, aErr := strconv.ParseUint(a.pre[i], 10, 64)
		bn, bErr := strconv.ParseUint(b.pre[i], 10, 64)
		var c int
		switch {
		case aErr == nil && bErr == nil:
			c = cmp.Compare(an, bn)
		case aErr == nil:
			c = -1
		case bErr == nil:
			c = 1
		default:
			c = strings.Compare(a.pre[i], b.pre[i])
		}
		if c != 0 {
			return c
		}
	}
	return cmp.Compare(len(a.pre), len(b.pre))
}

// bound is one end of an interval. An unset bound is unbounded.
type bound struct {
	v         version
	inclusive bool
	set       bool
}

type interval struct{ lo, hi bound }

func (i interval) empty() bool {
	if !i.lo.set || !i.hi.set {
		return false
	}
	c := compareVersions(i.lo.v, i.hi.v)
	return c > 0 || c == 0 && !(i.lo.inclusive && i.hi.inclusive)
}

func (i interval) intersect(j interval) interval {
	return interval{lo: tighter(i.lo, j.lo, 1), hi: tighter(i.hi, j.hi, -1)}
}

// tighter returns the bound that excludes more: the greater lower bound
// (sign 1) or the lesser upper bound (sign -1).
func tighter(a, b bound, sign int) bound {
	switch {
	case !a.set:
		return b
	case !b.set:
		return a
	}
	if c := compareVersions(a.v, b.v) * sign; c != 0 {
		if c > 0 {
			return a
		}
		return b
	}
	if !a.inclusive {
		return a
	}
	return b
}

// versionRange is the set of versions in any of its intervals.
type versionRange []interval

func (r versionRange) overlaps(s versionRange) bool {
	for _, i := range r {
		for _, j := range s {
			if !i.intersect(j).empty() {
				return true
			}
		}
	}
	return false
}

// parseVersionRange parses a version or range loosely. A value such as
// actions/checkout@v4 or pnpm@9.1.0 is read from after its last @, pnpm
// lockfiles' peer suffix in 18.2.0(react@18.2.0) is dropped, and so are the
// npm: and workspace: protocols. Alternatives are separated by || or, as in
// Composer, |. A version with fewer than three parts, such as v4 or 18.x, is
// the range of versions that start with it.
func parseVersionRange(s string) (versionRange, bool) {
	if i := strings.IndexByte(s, '('); i >= 0 {
		s = s[:i]
	}
	if i := strings.LastIndexByte(s, '@'); i >= 0 {
		s = s[i+1:]
	}
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "npm:")
	s = strings.TrimPrefix(s, "workspace:")
	var r versionRange
	for _, alt := range strings.Split(strings.ReplaceAll(s, "||", "|"), "|") {
		i, ok := parseComparatorSet(alt)
		if !ok {
			return nil, false
		}
		r = append(r, i)
	}
	return r, true
}

// parseComparatorSet parses comparators separated by spaces or commas, which
// all must hold, or a hyphen range such as 1.2 - 1.4.
func parseComparatorSet(s string) (interval, bool) {
	var fields []string
	pendingOp := ""
	for _, f := range strings.FieldsFunc(s, func(r rune) bool { return r == ' ' || r == '\t' || r == ',' }) {
		// Join an operator written apart from its version, as in ">= 1.2".
		if strings.Trim(f, "<>=^~") == "" {
			pendingOp += f
			continue
		}
		fields = append(fields, pendingOp+f)
		pendingOp = ""
	}
	if pendingOp != "" || len(fields) == 0 {
		return interval{}, false
	}

	if len(fields) == 3 && fields[1] == "-" {
		from, ok1 := parsePartial(fields[0])
		to, ok2 := parsePartial(fields[2])
		if !ok1 || !ok2 {
			return interval{}, false
		}
		var i interval
		if from.n > 0 {
			i.lo = bound{v: from.floor(), inclusive: true, set: true}
		}
		if to.n == 3 {
			i.hi = bound{v: to.floor(), inclusive: true, set: true}
		} else if to.n > 0 {
			i.hi = bound{v: to.next(), set: true}
		}
		return i, true
	}

	var res interval
	for _, f := range fields {
		i, ok := parseComparator(f)
		if !ok {
			return interval{}, false
		}
		res = res.intersect(i)
	}
	return res, true
}

// parseComparator parses an operator (<, <=, >, >=, =, ==, ^ or ~, or none)
// followed by a version, with the meanings npm gives them.
func parseComparator(s string) (interval, bool) {
	op := ""
	for _, o := range []string{"<=", ">=", "==", "<", ">", "=", "^", "~"} {
		if strings.HasPrefix(s, o) {
			op, s = o, s[len(o):]
			break
		}
	}
	p, ok := parsePartial(s)
	if !ok {
		return interval{}, false
	}
	floor := bound{v: p.floor(), inclusive: true, set: true}
	next := bound{v: p.next(), set: p.n < 3 && p.n > 0}
	nothing := interval{lo: floor, hi: bound{v: p.floor(), set: true}}
	switch op {
	case "", "=", "==":
		if p.n == 3 {
			return interval{lo: floor, hi: floor}, true
		}
		if p.n == 0 {
			return interval{}, true
		}
		return interval{lo: floor, hi: next}, true
	case ">":
		if p.n == 3 {
			return interval{lo: bound{v: p.floor(), set: true}}, true
		}
		if p.n == 0 {
			return nothing, true
		}
		next.inclusive = true
		return interval{lo: next}, true
	case ">=":
		if p.n == 0 {
			return interval{}, true
		}
		return interval{lo: floor}, true
	case "<":
		if p.n == 0 {
			return nothing, true
		}
		hi := bound{v: p.floor(), set: true}
		if p.n < 3 {
			hi.v.pre = []string{"0"}
		}
		return interval{hi: hi}, true
	case "<=":
		if p.n == 3 {
			return interval{hi: floor}, true
		}
		return interval{hi: next}, true
	case "^":
		if p.n == 0 {
			return interval{}, true
		}
		var hi version
		switch {
		case p.nums[0] > 0 || p.n == 1:
			hi.nums = [3]uint64{p.nums[0] + 1}
		case p.nums[1] > 0 || p.n == 2:
			hi.nums = [3]uint64{0, p.nums[1] + 1}
		default:
			hi.nums = [3]uint64{0, 0, p.nums[2] + 1}
		}
		hi.pre = []string{"0"}
		return interval{lo: floor, hi: bound{v: hi, set: true}}, true
	case "~":
		if p.n == 0 {
			return interval{}, true
		}
		hi := version{nums: [3]uint64{p.nums[0] + 1}, pre: []string{"0"}}
		if p.n >= 2 {
			hi.nums = [3]uint64{p.nums[0], p.nums[1] + 1}
		}
		return interval{lo: floor, hi: bound{v: hi, set: true}}, true
	}
	return interval{}, false
}

// partial is a version that may stop early or end in a wildcard, such as 1,
// v1.2, 1.x or *. n is the number of numeric parts before the first
// wildcard or the end.
type partial struct {
	nums [3]uint64
	n    int
	pre  []string
}

// floor is the lowest version that starts with p.
func (p partial) floor() version {
	return version{nums: p.nums, pre: p.pre}
}

// next is the lowest version above every version that starts with p, for
// 0 < n < 3: 2.0.0-0 for 1 and 1.3.0-0 for 1.2.
func (p partial) next() version {
	v := version{pre: []string{"0"}}
	if p.n == 1 {
		v.nums = [3]uint64{p.nums[0] + 1}
	} else {
		v.nums = [3]uint64{p.nums[0], p.nums[1] + 1}
	}
	return v
}

// parsePartial parses an optional v, up to three numeric or wildcard (x, X
// or *) parts, and, after three numeric parts, an optional prerelease and
// build metadata. The prerelease may follow without a hyphen when it starts
// with a letter, as in 3.12.0b1.
func parsePartial(s string) (partial, bool) {
	var p partial
	s = strings.TrimLeft(s, "vV")
	wildcard := false
	for i := 0; i < 3; i++ {
		if s != "" && (s[0] == 'x' || s[0] == 'X' || s[0] == '*') {
			wildcard = true
			s = s[1:]
		} else {
			end := 0
			for end < len(s) && s[end] >= '0' && s[end] <= '9' {
				end++
			}
			if end == 0 {
				return p, false
			}
			// 63 bits so that the next major or minor does not overflow.
			num, err := strconv.ParseUint(s[:end], 10, 63)
			if err != nil {
				return p, false
			}
			s = s[end:]
			if !wildcard {
				p.nums[i] = num
				p.n = i + 1
			}
		}
		if i == 2 || s == "" || s[0] != '.' {
			break
		}
		s = s[1:]
	}

	if build := strings.IndexByte(s, '+'); build >= 0 {
		if !validIdentifiers(s[build+1:]) {
			return p, false
		}
		s = s[:build]
	}
	if s == "" {
		return p, true
	}
	if p.n < 3 {
		return p, false
	}
	if s[0] == '-' {
		s = s[1:]
	} else if !isLetter(s[0]) {
		return p, false
	}
	if !validIdentifiers(s) {
		return p, false
	}
	p.pre = strings.Split(s, ".")
	return p, true
}

func isLetter(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

// validIdentifiers reports whether s is dot-separated non-empty identifiers
// of letters, digits and hyphens.
func validIdentifiers(s string) bool {
	for _, id := range strings.Split(s, ".") {
		if id == "" || strings.ContainsFunc(id, func(r rune) bool {
			return !(r < 128 && (isLetter(byte(r)) || r >= '0' && r <= '9' || r == '-'))
		}) {
			return false
		}
	}
	return true
}
