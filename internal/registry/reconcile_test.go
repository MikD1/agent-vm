package registry

import "testing"

func TestReconcileStates(t *testing.T) {
	records := []Record{{Name: "managed"}, {Name: "orphaned"}}
	states := map[string]string{"managed": "running", "unmanaged": "stopped"}
	entries := ReconcileStates(records, states)

	got := map[string]Status{}
	state := map[string]string{}
	for _, e := range entries {
		got[e.Name] = e.Status
		state[e.Name] = e.State
	}
	if got["managed"] != StatusManaged {
		t.Errorf("managed = %q", got["managed"])
	}
	if got["orphaned"] != StatusOrphaned {
		t.Errorf("orphaned = %q", got["orphaned"])
	}
	if got["unmanaged"] != StatusUnmanaged {
		t.Errorf("unmanaged = %q", got["unmanaged"])
	}
	if state["managed"] != "running" {
		t.Errorf("managed state = %q", state["managed"])
	}
	if state["orphaned"] != "-" {
		t.Errorf("orphaned state = %q", state["orphaned"])
	}
	if state["unmanaged"] != "stopped" {
		t.Errorf("unmanaged state = %q", state["unmanaged"])
	}
	if len(entries) != 3 {
		t.Errorf("want 3 entries, got %d", len(entries))
	}
}

func TestReconcileNamesOnlyUsesUnknownStateForLiveVMs(t *testing.T) {
	entries := Reconcile([]Record{{Name: "managed"}}, []string{"managed", "unmanaged"})
	state := map[string]string{}
	for _, e := range entries {
		state[e.Name] = e.State
	}
	if state["managed"] != "-" {
		t.Errorf("managed state = %q", state["managed"])
	}
	if state["unmanaged"] != "-" {
		t.Errorf("unmanaged state = %q", state["unmanaged"])
	}
}
