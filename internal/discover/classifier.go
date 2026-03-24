package discover

import (
	"fmt"

	"github.com/depscope/depscope/internal/manifest"
)

func Classify(pkg manifest.Package, compromised Range, offline bool) ProjectMatch {
	match := ProjectMatch{Depth: "direct"}
	if pkg.Depth > 1 {
		match.Depth = "transitive"
		match.DependencyPath = append(pkg.Parents, pkg.Name)
	}

	if pkg.ResolvedVersion != "" {
		match.Version = pkg.ResolvedVersion
		v, err := ParseVersion(pkg.ResolvedVersion)
		if err != nil {
			match.Status = StatusUnresolvable
			match.Reason = fmt.Sprintf("cannot parse version %q: %s", pkg.ResolvedVersion, err)
			return match
		}
		if compromised.Contains(v) {
			match.Status = StatusConfirmed
			match.Reason = fmt.Sprintf("resolved version %s is in compromised range", pkg.ResolvedVersion)
		} else {
			match.Status = StatusSafe
			match.Reason = fmt.Sprintf("resolved version %s is outside compromised range", pkg.ResolvedVersion)
		}
		return match
	}

	if pkg.Constraint != "" {
		match.Constraint = pkg.Constraint
		overlaps, err := ConstraintOverlaps(pkg.Constraint, compromised)
		if err != nil {
			match.Status = StatusUnresolvable
			match.Reason = fmt.Sprintf("cannot parse constraint %q: %s", pkg.Constraint, err)
			return match
		}
		if overlaps {
			match.Status = StatusPotentially
			match.Reason = fmt.Sprintf("constraint %s allows compromised versions", pkg.Constraint)
		} else {
			match.Status = StatusSafe
			match.Reason = fmt.Sprintf("constraint %s excludes compromised range", pkg.Constraint)
		}
		return match
	}

	if offline {
		match.Status = StatusUnresolvable
		match.Reason = "no version constraint, no lockfile, offline mode"
	} else {
		match.Status = StatusUnresolvable
		match.Reason = "no version constraint, no lockfile"
	}
	return match
}
