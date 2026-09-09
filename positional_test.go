package pinner

import (
	"strings"
	"testing"
)

// TestMapPositionalArgs covers the canonical positional->named-arg mapping
// rule in isolation: right-aligned to declared <arg> slots, optional leading
// slot omission, surplus rejection, and no-clobber of flag-populated values.
func TestMapPositionalArgs(t *testing.T) {
	cases := []struct {
		name       string
		args       []OperationArg
		pos        string
		supplied   []string
		input      map[string]any
		wantInput  map[string]any
		wantErrSub string // non-empty => expect an error containing this substring
	}{
		{
			name:      "single required slot",
			args:      []OperationArg{{Name: "domain", Type: ArgTypeString, Required: true}},
			pos:       "<domain>",
			supplied:  []string{"example.com"},
			wantInput: map[string]any{"domain": "example.com"},
		},
		{
			name:      "optional-lead omitted maps to trailing slot",
			args:      []OperationArg{{Name: "website", Type: ArgTypeString}, {Name: "domain", Type: ArgTypeString, Required: true}},
			pos:       "[<website>] <domain>",
			supplied:  []string{"example.com"},
			wantInput: map[string]any{"domain": "example.com"},
		},
		{
			name:      "optional lead populated",
			args:      []OperationArg{{Name: "website", Type: ArgTypeString}, {Name: "domain", Type: ArgTypeString, Required: true}},
			pos:       "[<website>] <domain>",
			supplied:  []string{"my-site", "example.com"},
			wantInput: map[string]any{"website": "my-site", "domain": "example.com"},
		},
		{
			name:      "placeholder label resolves to website arg",
			args:      []OperationArg{{Name: "website", Type: ArgTypeString}},
			pos:       "<domain>", // user-facing label, drives the "website" arg
			supplied:  []string{"example.com"},
			wantInput: map[string]any{"website": "example.com"},
		},
		{
			name:      "no args",
			args:      []OperationArg{{Name: "domain", Type: ArgTypeString, Required: true}},
			pos:       "<domain>",
			supplied:  nil,
			wantInput: map[string]any{},
		},
		{
			name:       "surplus rejected and names the extra arg",
			args:       []OperationArg{{Name: "domain", Type: ArgTypeString, Required: true}},
			pos:        "<domain>",
			supplied:   []string{"good.example", "bogus"},
			wantErrSub: "bogus",
		},
		{
			name:       "surplus rejected for two-slot op",
			args:       []OperationArg{{Name: "website", Type: ArgTypeString}, {Name: "domain", Type: ArgTypeString, Required: true}},
			pos:        "[<website>] <domain>",
			supplied:   []string{"my-site", "good.example", "bogus"},
			wantErrSub: "bogus",
		},
		{
			name:       "flag and positional conflict is rejected",
			args:       []OperationArg{{Name: "website", Type: ArgTypeString}, {Name: "domain", Type: ArgTypeString, Required: true}},
			pos:        "[<website>] <domain>",
			supplied:   []string{"positional-site", "example.com"},
			input:      map[string]any{"website": "flag-site"},
			wantErrSub: "website",
		},
		{
			name:      "empty positional declaration ignores args",
			args:      []OperationArg{{Name: "domain", Type: ArgTypeString, Required: true}},
			pos:       "",
			supplied:  []string{"x"},
			wantInput: map[string]any{},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			input := c.input
			if input == nil {
				input = map[string]any{}
			}
			err := MapPositionalArgs(c.args, c.pos, c.supplied, input)
			if c.wantErrSub != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", c.wantErrSub)
				}
				if !strings.Contains(err.Error(), c.wantErrSub) {
					t.Fatalf("error %q does not contain %q", err.Error(), c.wantErrSub)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			for k, want := range c.wantInput {
				if input[k] != want {
					t.Errorf("input[%q] = %v, want %v", k, input[k], want)
				}
			}
			if len(c.wantInput) != len(input) {
				t.Errorf("input has unexpected extra keys: %v", input)
			}
		})
	}
}

// TestMapPositionalArgsDuplicateSlotFallback pins the duplicate-slot guard:
// when two <placeholder> slots both fall through to the same named arg (the
// placeholder->first-string-arg fallback), mapping must fail with a collision
// diagnostic rather than silently overwrite the first slot's value or return
// the unrelated flag-conflict error. The arg is deliberately left unpopulated
// in input so the flag-conflict path cannot fire instead.
func TestMapPositionalArgsDuplicateSlotFallback(t *testing.T) {
	args := []OperationArg{{Name: "target", Type: ArgTypeString}}
	input := map[string]any{}
	err := MapPositionalArgs(args, "<a> <b>", []string{"one", "two"}, input)
	if err == nil {
		t.Fatalf("expected an error for two slots mapping to the same arg, got nil; input = %v", input)
	}
	if strings.Contains(err.Error(), "provided both as a flag") {
		t.Fatalf("got the flag-conflict error %q, want the duplicate-slot collision error", err.Error())
	}
	if !strings.Contains(err.Error(), "already bound") {
		t.Fatalf("error %q does not indicate a slot collision (want %q substring)", err.Error(), "already bound")
	}
	// The error aborts mapping: the second slot must not overwrite the first
	// slot's value (no silent last-write-wins "two").
	if got := input["target"]; got != "one" {
		t.Errorf("input[\"target\"] = %v, want the first slot's %q (second slot must not overwrite)", got, "one")
	}
}
