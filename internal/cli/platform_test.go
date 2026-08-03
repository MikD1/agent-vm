package cli

import "testing"

func TestParseHostPlatform(t *testing.T) {
	tests := []struct {
		goos    string
		want    hostPlatform
		wantErr bool
	}{
		{goos: "darwin", want: hostMacOS},
		{goos: "linux", want: hostLinux},
		{goos: "windows", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.goos, func(t *testing.T) {
			got, err := parseHostPlatform(tt.goos)
			if tt.wantErr {
				if err == nil {
					t.Fatal("want error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Fatalf("platform = %q, want %q", got, tt.want)
			}
		})
	}
}
