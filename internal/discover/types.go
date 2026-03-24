package discover

type Status int

const (
	StatusConfirmed    Status = iota
	StatusPotentially
	StatusUnresolvable
	StatusSafe
)

func (s Status) String() string {
	switch s {
	case StatusConfirmed:
		return "confirmed"
	case StatusPotentially:
		return "potentially"
	case StatusUnresolvable:
		return "unresolvable"
	case StatusSafe:
		return "safe"
	default:
		return "unknown"
	}
}

type ProjectMatch struct {
	Project        string
	Status         Status
	Source         string
	Version        string
	Constraint     string
	Depth          string
	DependencyPath []string
	Reason         string
}

type DiscoverResult struct {
	Package string
	Range   string
	Matches []ProjectMatch
}

type ResultSummary struct {
	Confirmed    int `json:"confirmed"`
	Potentially  int `json:"potentially"`
	Unresolvable int `json:"unresolvable"`
	Safe         int `json:"safe"`
	Total        int `json:"total"`
}

func (r *DiscoverResult) Summary() ResultSummary {
	var s ResultSummary
	for _, m := range r.Matches {
		switch m.Status {
		case StatusConfirmed:
			s.Confirmed++
		case StatusPotentially:
			s.Potentially++
		case StatusUnresolvable:
			s.Unresolvable++
		case StatusSafe:
			s.Safe++
		}
	}
	s.Total = len(r.Matches)
	return s
}

type Config struct {
	Package   string
	Range     string
	StartPath string
	ListFile  string
	Ecosystem string
	MaxDepth  int
	Resolve   bool
	Offline   bool
	Output    string
}
