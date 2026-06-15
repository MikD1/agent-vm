// Package vmname normalizes and validates VM names as Lima DNS labels.
package vmname

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	nonLabel = regexp.MustCompile(`[^a-z0-9-]`)
	dnsLabel = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`)
)

// Normalize lowercases raw, replaces every char outside [a-z0-9-] with '-',
// then strips leading/trailing '-'. Returns an error if nothing valid remains.
func Normalize(raw string) (string, error) {
	n := nonLabel.ReplaceAllString(strings.ToLower(raw), "-")
	n = strings.Trim(n, "-")
	if n == "" {
		return "", fmt.Errorf("cannot derive a valid VM name from %q", raw)
	}
	return n, nil
}

// Validate asserts name is a lowercase DNS label (Lima's requirement).
func Validate(name string) error {
	if !dnsLabel.MatchString(name) {
		return fmt.Errorf("VM name must be a lowercase DNS label (a-z, 0-9, hyphen; not leading/trailing '-'): %q", name)
	}
	return nil
}
