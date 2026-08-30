package httpapi

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDomainActivityRejectsUnknownIDsAndHasNoErrorField(t *testing.T) {
	var a domainActivity
	a.Start("apply", []string{"discover_zone", "not-a-real-step", "create_tunnel"})
	a.Advance("discover_zone")
	a.FailCurrent()
	snap, ok := a.Snapshot()
	if !ok {
		t.Fatal("expected snapshot")
	}
	if snap["operation"] != "apply" {
		t.Fatalf("operation = %v", snap["operation"])
	}
	if snap["in_progress"] != false {
		t.Fatal("failed activity should not be in progress")
	}
	raw, err := json.Marshal(snap)
	if err != nil {
		t.Fatal(err)
	}
	body := string(raw)
	if strings.Contains(body, "not-a-real-step") {
		t.Fatalf("unknown step leaked: %s", body)
	}
	if strings.Contains(body, `"error"`) || strings.Contains(body, "token") || strings.Contains(body, "@") {
		t.Fatalf("snapshot must not carry errors or secrets: %s", body)
	}
	if !strings.Contains(body, `"Looking up the zone"`) || !strings.Contains(body, `"failed"`) {
		t.Fatalf("expected allow-listed failed step: %s", body)
	}
}

func TestDomainActivityStartIgnoresUnknownOperation(t *testing.T) {
	var a domainActivity
	a.Start("journald", []string{"discover_zone"})
	if _, ok := a.Snapshot(); ok {
		t.Fatal("unknown operation must not snapshot")
	}
}

func TestDomainActivitySucceedMarksAllDone(t *testing.T) {
	var a domainActivity
	a.Start("apply", []string{"discover_zone", "create_tunnel"})
	a.Advance("discover_zone")
	a.Succeed()
	snap, ok := a.Snapshot()
	if !ok {
		t.Fatal("expected snapshot")
	}
	if snap["in_progress"] != false {
		t.Fatal("succeeded activity still in progress")
	}
	steps, _ := snap["steps"].([]domainActivityStep)
	if len(steps) != 2 {
		t.Fatalf("steps = %d", len(steps))
	}
	for _, step := range steps {
		if step.State != domainActivityDone {
			t.Fatalf("step %s state = %s", step.ID, step.State)
		}
		if step.Label != domainActivityLabels[step.ID] {
			t.Fatalf("label drifted: %q", step.Label)
		}
	}
}
