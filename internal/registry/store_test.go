package registry

import (
	"testing"
	"time"

	"github.com/MikD1/agent-vm/internal/config"
)

func sampleRecord() Record {
	return Record{
		Name:      "my-api",
		Source:    "cli",
		CreatedAt: time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC),
		Base:      config.Base{Image: "template:_images/ubuntu"},
		Modules:   []string{"node", "claude"},
		Resources: config.Resources{CPUs: 4, Memory: "8GiB", Disk: "120GiB"},
		User:      "me",
		Workspace: config.Workspace{Mode: "clone", GuestPath: "/home/me.linux/my-api", Repo: "git@h:acme/my-api.git", Ref: "main"},
	}
}

func TestStoreRoundTrip(t *testing.T) {
	s := NewStore(t.TempDir())
	rec := sampleRecord()
	if err := s.Write(rec); err != nil {
		t.Fatal(err)
	}
	ok, err := s.Exists("my-api")
	if err != nil || !ok {
		t.Fatalf("Exists = %v, %v", ok, err)
	}
	got, err := s.Read("my-api")
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != rec.Name || got.Workspace.Repo != rec.Workspace.Repo || got.Modules[1] != "claude" {
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
