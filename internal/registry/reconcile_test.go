package registry

import "testing"

func TestReconcile(t *testing.T) {
	records := []Record{{Name: "managed"}, {Name: "orphaned"}}
	limaNames := []string{"managed", "unmanaged"}
	entries := Reconcile(records, limaNames)

	got := map[string]Status{}
	for _, e := range entries {
		got[e.Name] = e.Status
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
	if len(entries) != 3 {
		t.Errorf("want 3 entries, got %d", len(entries))
	}
}
