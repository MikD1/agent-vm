package registry

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/MikD1/agent-vm/internal/config"
)

func sampleRecord() Record {
	return Record{
		Name:      "work",
		CreatedAt: time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC),
		Base:      config.Base{Image: "template:_images/ubuntu"},
		Modules:   []config.ModuleSpec{{Name: "node"}, {Name: "claude"}},
		Resources: config.Resources{CPUs: 4, Memory: "8GiB", Disk: "120GiB"},
		User:      "me",
		ConfigDir: "/Users/me/vms/work",
		Home:      "/home/me.linux",
	}
}

func TestStoreRoundTrip(t *testing.T) {
	s := NewStore(t.TempDir())
	rec := sampleRecord()
	if err := s.Write(rec); err != nil {
		t.Fatal(err)
	}
	ok, err := s.Exists("work")
	if err != nil || !ok {
		t.Fatalf("Exists = %v, %v", ok, err)
	}
	got, err := s.Read("work")
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != rec.Name || got.ConfigDir != rec.ConfigDir || got.Home != rec.Home || got.Modules[1].Name != "claude" {
		t.Errorf("round-trip mismatch: %+v", got)
	}
}

func TestStoreListAndDelete(t *testing.T) {
	s := NewStore(t.TempDir())
	_ = s.Write(sampleRecord())
	r2 := sampleRecord()
	r2.Name = "other"
	_ = s.Write(r2)
	list, err := s.List()
	if err != nil || len(list) != 2 {
		t.Fatalf("List = %d records, %v", len(list), err)
	}
	if err := s.Delete("other"); err != nil {
		t.Fatal(err)
	}
	ok, _ := s.Exists("other")
	if ok {
		t.Error("record should be gone after Delete")
	}
}

func TestReadMissing(t *testing.T) {
	s := NewStore(t.TempDir())
	if _, err := s.Read("ghost"); err == nil {
		t.Error("want error reading missing record")
	}
}

func TestStoreRoundTripMounts(t *testing.T) {
	s := NewStore(t.TempDir())
	rec := sampleRecord()
	rec.Mounts = []config.Mount{
		{HostPath: "/Users/me/shared-lib", GuestPath: "/home/me.linux/shared-lib"},
	}
	if err := s.Write(rec); err != nil {
		t.Fatal(err)
	}
	got, err := s.Read("work")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Mounts) != 1 || got.Mounts[0].GuestPath != "/home/me.linux/shared-lib" {
		t.Errorf("mounts round-trip mismatch: %+v", got.Mounts)
	}
}

// TestRecordKeepsEmptyMounts proves an empty mount list survives the round trip
// as an explicit empty list rather than vanishing from the file.
func TestRecordKeepsEmptyMounts(t *testing.T) {
	s := NewStore(t.TempDir())
	rec := sampleRecord()
	if err := s.Write(rec); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(s.vmsDir(), "work.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "mounts:") {
		t.Errorf("an empty mounts list must still be written:\n%s", data)
	}
	if strings.Contains(string(data), "workspace:") {
		t.Errorf("record must not carry a workspace key:\n%s", data)
	}
}
