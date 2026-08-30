package httpapi

import "sync"

const (
	domainActivityPending = "pending"
	domainActivityRunning = "running"
	domainActivityDone    = "done"
	domainActivityFailed  = "failed"
)

var domainActivityOperations = map[string]struct{}{
	"apply":          {},
	"manual_apply":   {},
	"access_policy":  {},
	"confirm_access": {},
	"tls":            {},
	"confirm":        {},
	"disconnect":     {},
}

var domainActivityLabels = map[string]string{
	"discover_zone":      "Looking up the zone",
	"create_tunnel":      "Creating the tunnel",
	"write_dns":          "Writing DNS records",
	"create_access":      "Creating Access applications",
	"store_connector":    "Storing the connector on the server",
	"queue_tls":          "Queuing certificates",
	"save_config":        "Saving the domain record",
	"write_instructions": "Preparing DNS instructions",
	"access_policy":      "Adding the Access allow policy",
	"confirm_access":     "Recording that Access is configured",
	"issue_tls":          "Issuing certificates",
	"close_bootstrap":    "Closing the bootstrap listener",
	"disconnect":         "Removing domain resources",
}

type domainActivityStep struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	State string `json:"state"`
}

// domainActivity is in-memory, secret-safe progress for Domain mutations.
// Labels come only from domainActivityLabels. It never stores tokens, emails,
// hostnames, zone names, or raw dependency errors.
type domainActivity struct {
	mu         sync.Mutex
	operation  string
	inProgress bool
	steps      []domainActivityStep
}

func (a *domainActivity) Start(operation string, ids []string) {
	if _, ok := domainActivityOperations[operation]; !ok {
		return
	}
	steps := make([]domainActivityStep, 0, len(ids))
	for _, id := range ids {
		label, ok := domainActivityLabels[id]
		if !ok {
			continue
		}
		steps = append(steps, domainActivityStep{ID: id, Label: label, State: domainActivityPending})
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.operation = operation
	a.inProgress = len(steps) > 0
	a.steps = steps
}

func (a *domainActivity) Advance(id string) {
	if _, ok := domainActivityLabels[id]; !ok {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	for i := range a.steps {
		if a.steps[i].ID == id {
			for j := 0; j < i; j++ {
				if a.steps[j].State == domainActivityRunning || a.steps[j].State == domainActivityPending {
					a.steps[j].State = domainActivityDone
				}
			}
			a.steps[i].State = domainActivityRunning
			return
		}
	}
}

func (a *domainActivity) Succeed() {
	a.mu.Lock()
	defer a.mu.Unlock()
	for i := range a.steps {
		if a.steps[i].State == domainActivityPending || a.steps[i].State == domainActivityRunning {
			a.steps[i].State = domainActivityDone
		}
	}
	a.inProgress = false
}

func (a *domainActivity) FailCurrent() {
	a.mu.Lock()
	defer a.mu.Unlock()
	failed := false
	for i := range a.steps {
		if a.steps[i].State == domainActivityRunning {
			a.steps[i].State = domainActivityFailed
			failed = true
			break
		}
	}
	if !failed {
		for i := range a.steps {
			if a.steps[i].State == domainActivityPending {
				a.steps[i].State = domainActivityFailed
				break
			}
		}
	}
	a.inProgress = false
}

func (a *domainActivity) Clear() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.operation = ""
	a.inProgress = false
	a.steps = nil
}

func (a *domainActivity) Snapshot() (map[string]any, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.operation == "" || len(a.steps) == 0 {
		return nil, false
	}
	steps := make([]domainActivityStep, len(a.steps))
	copy(steps, a.steps)
	return map[string]any{
		"operation":   a.operation,
		"in_progress": a.inProgress,
		"steps":       steps,
	}, true
}

func (s *Server) beginDomainActivity(operation string, ids ...string) func(*bool) {
	s.domainActivity.Start(operation, ids)
	if len(ids) > 0 {
		s.domainActivity.Advance(ids[0])
	}
	return func(ok *bool) {
		if ok != nil && *ok {
			s.domainActivity.Succeed()
			return
		}
		s.domainActivity.FailCurrent()
	}
}

func (s *Server) attachDomainActivity(resp map[string]any) {
	snap, ok := s.domainActivity.Snapshot()
	if !ok {
		return
	}
	resp["activity"] = snap
}
