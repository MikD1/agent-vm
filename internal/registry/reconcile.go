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
	Record *Record
}

// Reconcile cross-references the registry against Lima's existing VM names.
func Reconcile(records []Record, limaNames []string) []Entry {
	live := map[string]bool{}
	for _, n := range limaNames {
		live[n] = true
	}
	known := map[string]bool{}
	var out []Entry
	for i := range records {
		r := records[i]
		known[r.Name] = true
		st := StatusOrphaned
		if live[r.Name] {
			st = StatusManaged
		}
		out = append(out, Entry{Name: r.Name, Status: st, Record: &records[i]})
	}
	for _, n := range limaNames {
		if !known[n] {
			out = append(out, Entry{Name: n, Status: StatusUnmanaged})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
