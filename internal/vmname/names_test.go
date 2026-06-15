package vmname

import "testing"

func TestNormalize(t *testing.T) {
	cases := []struct {
		in, want string
		wantErr  bool
	}{
		{"my-api", "my-api", false},
		{"My_Project", "my-project", false},
		{"API.v2", "api-v2", false},
		{"--lead-trail--", "lead-trail", false},
		{"_____", "", true}, // nothing valid remains
		{"", "", true},
	}
	for _, c := range cases {
		got, err := Normalize(c.in)
		if c.wantErr {
			if err == nil {
				t.Errorf("Normalize(%q): want error, got %q", c.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("Normalize(%q): unexpected error %v", c.in, err)
		}
		if got != c.want {
			t.Errorf("Normalize(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestValidate(t *testing.T) {
	valid := []string{"a", "my-api", "node24", "a-b-c"}
	invalid := []string{"", "-x", "x-", "My-API", "a_b", "a.b"}
	for _, v := range valid {
		if err := Validate(v); err != nil {
			t.Errorf("Validate(%q): unexpected error %v", v, err)
		}
	}
	for _, v := range invalid {
		if err := Validate(v); err == nil {
			t.Errorf("Validate(%q): want error, got nil", v)
		}
	}
}
