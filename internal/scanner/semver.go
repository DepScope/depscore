package scanner

import (
	"fmt"
	"strconv"
	"strings"
)

// semversion represents a parsed semantic version (major.minor.patch).
type semversion struct {
	major int
	minor int
	patch int
}

// compare returns -1, 0, or 1 depending on whether v is less than, equal to,
// or greater than other.
func (v semversion) compare(other semversion) int {
	if v.major != other.major {
		if v.major < other.major {
			return -1
		}
		return 1
	}
	if v.minor != other.minor {
		if v.minor < other.minor {
			return -1
		}
		return 1
	}
	if v.patch != other.patch {
		if v.patch < other.patch {
			return -1
		}
		return 1
	}
	return 0
}

func (v semversion) gte(other semversion) bool { return v.compare(other) >= 0 }
func (v semversion) gt(other semversion) bool  { return v.compare(other) > 0 }
func (v semversion) lt(other semversion) bool  { return v.compare(other) < 0 }
func (v semversion) eq(other semversion) bool  { return v.compare(other) == 0 }

// parseSemver parses a semver string like "1.14.1", "v1.14.1", or
// "1.0.0-beta.1+build.123". Pre-release and build metadata are stripped.
func parseSemver(s string) (semversion, error) {
	if s == "" {
		return semversion{}, fmt.Errorf("empty version string")
	}

	// Strip leading "v" or "=" prefix.
	s = strings.TrimPrefix(s, "v")
	s = strings.TrimPrefix(s, "=")

	// Strip pre-release suffix (everything after first '-').
	if idx := strings.IndexByte(s, '-'); idx != -1 {
		s = s[:idx]
	}
	// Strip build metadata (everything after first '+').
	if idx := strings.IndexByte(s, '+'); idx != -1 {
		s = s[:idx]
	}

	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return semversion{}, fmt.Errorf("invalid semver: %q", s)
	}

	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return semversion{}, fmt.Errorf("invalid major version: %w", err)
	}
	minor, err := strconv.Atoi(parts[1])
	if err != nil {
		return semversion{}, fmt.Errorf("invalid minor version: %w", err)
	}
	patch, err := strconv.Atoi(parts[2])
	if err != nil {
		return semversion{}, fmt.Errorf("invalid patch version: %w", err)
	}

	return semversion{major: major, minor: minor, patch: patch}, nil
}

// semverSatisfies checks whether version satisfies an npm-style semver
// constraint. Supported forms:
//
//   - Exact: "1.14.1"
//   - Caret: "^1.14.0" (compatible with version)
//   - Tilde: "~1.14.0" (approximately equivalent)
//   - Comparison: ">=1.14.0", ">1.14.0", "<=1.14.0", "<1.14.0"
//   - Compound (comma-separated): ">=0.30.0,<0.31.0"
//   - Wildcard: "*" or "latest"
//   - Explicit exact: "=1.14.1"
func semverSatisfies(constraint, version string) bool {
	constraint = strings.TrimSpace(constraint)
	version = strings.TrimSpace(version)

	// Wildcard matches everything.
	if constraint == "*" || constraint == "latest" {
		return true
	}

	// Compound constraints (comma-separated): all parts must match.
	if strings.Contains(constraint, ",") {
		parts := strings.Split(constraint, ",")
		for _, part := range parts {
			if !semverSatisfies(strings.TrimSpace(part), version) {
				return false
			}
		}
		return true
	}

	ver, err := parseSemver(version)
	if err != nil {
		return false
	}

	// Caret range: ^MAJOR.MINOR.PATCH
	if strings.HasPrefix(constraint, "^") {
		cv, err := parseSemver(constraint[1:])
		if err != nil {
			return false
		}
		if ver.lt(cv) {
			return false
		}
		if cv.major > 0 {
			// ^1.14.0 → >=1.14.0, <2.0.0
			upper := semversion{major: cv.major + 1, minor: 0, patch: 0}
			return ver.lt(upper)
		}
		if cv.minor > 0 {
			// ^0.MINOR.PATCH → >=0.MINOR.PATCH, <0.(MINOR+1).0
			upper := semversion{major: 0, minor: cv.minor + 1, patch: 0}
			return ver.lt(upper)
		}
		// ^0.0.PATCH → >=0.0.PATCH, <0.0.(PATCH+1)
		upper := semversion{major: 0, minor: 0, patch: cv.patch + 1}
		return ver.lt(upper)
	}

	// Tilde range: ~MAJOR.MINOR.PATCH
	if strings.HasPrefix(constraint, "~") {
		cv, err := parseSemver(constraint[1:])
		if err != nil {
			return false
		}
		if ver.lt(cv) {
			return false
		}
		// ~1.14.0 → >=1.14.0, <1.15.0
		upper := semversion{major: cv.major, minor: cv.minor + 1, patch: 0}
		return ver.lt(upper)
	}

	// Comparison operators: >=, >, <=, <
	if strings.HasPrefix(constraint, ">=") {
		cv, err := parseSemver(constraint[2:])
		if err != nil {
			return false
		}
		return ver.gte(cv)
	}
	if strings.HasPrefix(constraint, ">") {
		cv, err := parseSemver(constraint[1:])
		if err != nil {
			return false
		}
		return ver.gt(cv)
	}
	if strings.HasPrefix(constraint, "<=") {
		cv, err := parseSemver(constraint[2:])
		if err != nil {
			return false
		}
		return !ver.gt(cv)
	}
	if strings.HasPrefix(constraint, "<") {
		cv, err := parseSemver(constraint[1:])
		if err != nil {
			return false
		}
		return ver.lt(cv)
	}

	// Explicit exact: "=1.14.1" (the "=" is stripped by parseSemver)
	// or plain exact: "1.14.1"
	cv, err := parseSemver(constraint)
	if err != nil {
		return false
	}
	return ver.eq(cv)
}
