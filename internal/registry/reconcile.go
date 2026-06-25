package registry

import "sort"

// Status labels a reconciled VM.
type Status string

const (
	StatusManaged   Status = "managed"   // Record + VM both exist
	StatusOrphaned  Status = "orphaned"  // Record exists, VM does not
	StatusUnmanaged Status = "unmanaged" // VM exists, no Record
)

// Entry is one reconciled VM; Record is nil for unmanaged VMs.
type Entry struct {
	Name   string
	Status Status
	State  string
	Record *Record
}

// Reconcile cross-references the registry against existing Lima VM names.
func Reconcile(records []Record, limaNames []string) []Entry {
	states := map[string]string{}
	for _, n := range limaNames {
		states[n] = "-"
	}
	return ReconcileStates(records, states)
}

// ReconcileStates cross-references the registry against existing Lima VMs keyed by name.
func ReconcileStates(records []Record, limaStates map[string]string) []Entry {
	known := map[string]bool{}
	var out []Entry
	for i := range records {
		r := records[i]
		known[r.Name] = true
		st := StatusOrphaned
		state := "-"
		if liveState, ok := limaStates[r.Name]; ok {
			st = StatusManaged
			state = liveState
		}
		out = append(out, Entry{Name: r.Name, Status: st, State: state, Record: &records[i]})
	}
	for name, state := range limaStates {
		if !known[name] {
			out = append(out, Entry{Name: name, Status: StatusUnmanaged, State: state})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
