// internal/discover/version.go
package discover

import (
	"fmt"
	"strconv"
	"strings"
)

// Version represents a parsed semantic version.
type Version struct {
	Major int
	Minor int
	Patch int
	Pre   string // pre-release identifier (e.g., "rc1", "beta2")
}

// ParseVersion parses a version string like "1.82.7" or "v1.2.3".
func ParseVersion(s string) (Version, error) {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "v")
	if s == "" {
		return Version{}, fmt.Errorf("empty version string")
	}

	// Split off pre-release: "1.82.7rc1" → "1.82.7", "rc1"
	var pre string
	for i, c := range s {
		if c != '.' && (c < '0' || c > '9') {
			pre = s[i:]
			s = s[:i]
			break
		}
	}

	parts := strings.Split(s, ".")
	if len(parts) < 2 {
		return Version{}, fmt.Errorf("invalid version: %q", s)
	}

	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return Version{}, fmt.Errorf("invalid major version: %w", err)
	}
	minor, err := strconv.Atoi(parts[1])
	if err != nil {
		return Version{}, fmt.Errorf("invalid minor version: %w", err)
	}
	patch := 0
	if len(parts) >= 3 && parts[2] != "" {
		patch, err = strconv.Atoi(parts[2])
		if err != nil {
			return Version{}, fmt.Errorf("invalid patch version: %w", err)
		}
	}

	return Version{Major: major, Minor: minor, Patch: patch, Pre: pre}, nil
}

// Compare returns -1, 0, or 1.
func (v Version) Compare(other Version) int {
	if v.Major != other.Major {
		return cmpInt(v.Major, other.Major)
	}
	if v.Minor != other.Minor {
		return cmpInt(v.Minor, other.Minor)
	}
	if v.Patch != other.Patch {
		return cmpInt(v.Patch, other.Patch)
	}
	// Pre-release versions have lower precedence than release
	if v.Pre == "" && other.Pre != "" {
		return 1
	}
	if v.Pre != "" && other.Pre == "" {
		return -1
	}
	if v.Pre < other.Pre {
		return -1
	}
	if v.Pre > other.Pre {
		return 1
	}
	return 0
}

func cmpInt(a, b int) int {
	if a < b {
		return -1
	}
	return 1
}

// Constraint is a single version constraint like ">=1.82.7" or "<1.83.0".
type Constraint struct {
	Op      string  // ">=", "<=", ">", "<", "==", "!=", "~="
	Version Version
}

// Range is a set of constraints that must all be satisfied.
type Range struct {
	Constraints []Constraint
}

// ParseRange parses a range string like ">=1.82.7,<1.83.0".
func ParseRange(s string) (Range, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return Range{}, fmt.Errorf("empty range")
	}

	parts := strings.Split(s, ",")
	var constraints []Constraint
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		c, err := parseConstraint(part)
		if err != nil {
			return Range{}, fmt.Errorf("parsing constraint %q: %w", part, err)
		}
		constraints = append(constraints, c)
	}

	if len(constraints) == 0 {
		return Range{}, fmt.Errorf("no valid constraints in range %q", s)
	}

	return Range{Constraints: constraints}, nil
}

func parseConstraint(s string) (Constraint, error) {
	ops := []string{"~=", "===", "!==", "==", "!=", ">=", "<=", ">", "<"}
	for _, op := range ops {
		if strings.HasPrefix(s, op) {
			ver, err := ParseVersion(strings.TrimSpace(s[len(op):]))
			if err != nil {
				return Constraint{}, err
			}
			return Constraint{Op: op, Version: ver}, nil
		}
	}
	return Constraint{}, fmt.Errorf("no operator found in constraint %q", s)
}

// Contains returns true if the version satisfies all constraints in the range.
func (r Range) Contains(v Version) bool {
	for _, c := range r.Constraints {
		if !c.matches(v) {
			return false
		}
	}
	return true
}

func (c Constraint) matches(v Version) bool {
	cmp := v.Compare(c.Version)
	switch c.Op {
	case "==", "===":
		return cmp == 0
	case "!=", "!==":
		return cmp != 0
	case ">=":
		return cmp >= 0
	case "<=":
		return cmp <= 0
	case ">":
		return cmp > 0
	case "<":
		return cmp < 0
	case "~=":
		// Compatible release: ~=1.82.0 means >=1.82.0,<1.83.0
		if cmp < 0 {
			return false
		}
		return v.Major == c.Version.Major && v.Minor == c.Version.Minor
	default:
		return false
	}
}

// ConstraintOverlaps checks if a manifest constraint string allows any version
// that falls within the given range. Used for "potentially affected" classification.
func ConstraintOverlaps(constraint string, r Range) (bool, error) {
	constraint = strings.TrimSpace(constraint)
	if constraint == "" {
		// Unpinned — could be anything, so it overlaps
		return true, nil
	}

	// Parse the constraint as a Range itself
	cr, err := ParseRange(constraint)
	if err != nil {
		// Try as a single constraint with implicit ==
		if v, verr := ParseVersion(constraint); verr == nil {
			return r.Contains(v), nil
		}
		return false, err
	}

	// Compute the effective bounds of both ranges and check for intersection.
	rLow, rHigh := r.bounds()
	cLow, cHigh := cr.bounds()

	// If constraint's upper bound is at or below range's lower bound → no overlap.
	// When the constraint uses strict-less-than (<), the boundary is exclusive,
	// so cHigh == rLow also means no overlap.
	if cHigh != nil && rLow != nil {
		cmp := cHigh.Compare(*rLow)
		if cmp < 0 {
			return false, nil
		}
		if cmp == 0 && !cr.highInclusive() {
			// constraint's upper bound equals range's lower bound but is exclusive
			return false, nil
		}
	}

	// If constraint's lower bound is at or above range's upper bound → no overlap.
	if cLow != nil && rHigh != nil {
		cmp := cLow.Compare(*rHigh)
		if cmp > 0 {
			return false, nil
		}
		if cmp == 0 && !r.highInclusive() {
			// range's upper bound equals constraint's lower bound but is exclusive
			return false, nil
		}
	}

	// If we get here, the ranges could overlap
	return true, nil
}

// bounds returns the lower and upper bounds of a range (nil means unbounded).
func (r Range) bounds() (low *Version, high *Version) {
	for _, c := range r.Constraints {
		switch c.Op {
		case ">=", ">":
			v := c.Version
			if low == nil || v.Compare(*low) > 0 {
				low = &v
			}
		case "<=", "<":
			v := c.Version
			if high == nil || v.Compare(*high) < 0 {
				high = &v
			}
		case "==", "===":
			v := c.Version
			low = &v
			high = &v
		case "~=":
			v := c.Version
			low = &v
			upper := Version{Major: v.Major, Minor: v.Minor + 1, Patch: 0}
			high = &upper
		}
	}
	return
}

func (r Range) highInclusive() bool {
	for _, c := range r.Constraints {
		if c.Op == "<=" || c.Op == "==" {
			return true
		}
	}
	return false
}
