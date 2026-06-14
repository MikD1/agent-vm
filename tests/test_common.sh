#!/usr/bin/env bash
# Unit tests for lib/common.sh name helpers.
# Run: bash tests/test_common.sh
set -uo pipefail

REPO_DIR="$(cd "$(dirname "$0")/.." && pwd)"
# shellcheck source=../lib/common.sh
source "$REPO_DIR/lib/common.sh"

fail=0
check_eq() { # desc expected actual
  if [[ "$2" == "$3" ]]; then
    printf 'ok   - %s\n' "$1"
  else
    printf 'FAIL - %s: expected %q got %q\n' "$1" "$2" "$3"
    fail=1
  fi
}
check_true()  { if "$@" >/dev/null 2>&1; then printf 'ok   - %s\n' "$*"; else printf 'FAIL - %s (expected success)\n' "$*"; fail=1; fi; }
check_false() { if "$@" >/dev/null 2>&1; then printf 'FAIL - %s (expected failure)\n' "$*"; fail=1; else printf 'ok   - %s\n' "$*"; fi; }

# normalize_name runs in a subshell so its die()->exit can't kill the test run.
norm() { ( normalize_name "$1" ) 2>/dev/null; }

# --- normalize_name ---
check_eq "normalize lowercase+underscore" "my-project" "$(norm 'My_Project')"
check_eq "normalize strip edge hyphens"   "weird"      "$(norm '_weird__')"
check_eq "normalize dots to hyphens"      "a-b-c"      "$(norm 'a.b.c')"
check_eq "normalize already valid"        "api"        "$(norm 'api')"
check_eq "normalize trailing/leading mix" "my-project" "$(norm '-My-Project-')"
check_eq "normalize all-invalid -> empty" ""           "$(norm '@@@')"
check_false normalize_name '@@@'   # all-invalid must die (non-zero)

# --- validate_name ---
for good in my-project api a1 a; do check_true validate_name "$good"; done
for bad  in My_Project a_b -x x- '' 'a b'; do check_false validate_name "$bad"; done

# --- resolve_name_from_cwd normalizes the cwd basename ---
tmp="$(mktemp -d)/My_Project"
mkdir -p "$tmp"; : > "$tmp/.ai-dev-vm.yaml"
check_eq "resolve_name_from_cwd normalizes" "my-project" "$(cd "$tmp" && resolve_name_from_cwd)"

# --- resolve_target_name normalizes an explicit name ---
check_eq "resolve_target_name explicit normalizes" "my-project" "$(resolve_target_name 'My_Project')"

# --- validate_cpus ---
for good in 1 4 8 16; do check_true validate_cpus "$good"; done
for bad  in 0 8x '' 1.5 -2 abc; do check_false validate_cpus "$bad"; done

# --- validate_size (field-name arg + value arg) ---
for good in 16GiB 512MiB 120GiB 8G; do check_true validate_size memory "$good"; done
for bad  in 8x GiB '' '4 GiB' 16XB 4; do check_false validate_size memory "$bad"; done

if [[ "$fail" -eq 0 ]]; then echo "ALL PASS"; else echo "SOME FAILED"; fi
exit "$fail"
