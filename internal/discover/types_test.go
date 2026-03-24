package discover

import (
	"testing"
	"github.com/stretchr/testify/assert"
)

func TestStatusString(t *testing.T) {
	assert.Equal(t, "confirmed", StatusConfirmed.String())
	assert.Equal(t, "potentially", StatusPotentially.String())
	assert.Equal(t, "unresolvable", StatusUnresolvable.String())
	assert.Equal(t, "safe", StatusSafe.String())
}

func TestDiscoverResultSummary(t *testing.T) {
	result := &DiscoverResult{
		Package: "litellm",
		Range:   ">=1.82.7,<1.83.0",
		Matches: []ProjectMatch{
			{Project: "/a", Status: StatusConfirmed},
			{Project: "/b", Status: StatusConfirmed},
			{Project: "/c", Status: StatusPotentially},
			{Project: "/d", Status: StatusSafe},
		},
	}
	s := result.Summary()
	assert.Equal(t, 2, s.Confirmed)
	assert.Equal(t, 1, s.Potentially)
	assert.Equal(t, 0, s.Unresolvable)
	assert.Equal(t, 1, s.Safe)
	assert.Equal(t, 4, s.Total)
}
