package cli

import "testing"

func TestRootHasVerbosePersistentFlag(t *testing.T) {
	root := NewRootCmd()
	f := root.PersistentFlags().Lookup("verbose")
	if f == nil {
		t.Fatal("root is missing the persistent --verbose flag")
	}
	if f.DefValue != "false" {
		t.Errorf("--verbose default = %q, want %q", f.DefValue, "false")
	}
}
