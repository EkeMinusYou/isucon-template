package main

import (
	"strings"
	"testing"
)

func TestExtraServiceRolesRejectInvalidMapping(t *testing.T) {
	for _, mapping := range []string{"worker:missing", "worker", "worker:app:bad"} {
		err := runCheckRoles([]string{"-dry-run", "-role", "all=h1", "-role", "app=h1", "-role", "nginx=h1", "-role", "mysql=h1", "-var", "service=app", "-var", "extra_service_roles=" + mapping})
		if err == nil || !strings.Contains(err.Error(), "extra_service_roles") {
			t.Fatalf("mapping %q: got %v", mapping, err)
		}
	}
}

func TestExtraServiceRolesCheckAssignedAndUnassignedHosts(t *testing.T) {
	fake := &stampExecutor{}
	r := &deployRunner{
		exec: fake, parallel: 1,
		roles: roleValues{"all": {"h1", "h2"}, "app": {"h1"}, "nginx": {"h1"}, "mysql": {"h1"}},
		vars:  variableValues{"extra_service_roles": "worker:app,mock:app"},
	}
	if err := r.checkRoles("app"); err != nil {
		t.Fatal(err)
	}
	if len(fake.stdins) != 2 {
		t.Fatalf("expected checks on both hosts, got %d", len(fake.stdins))
	}
	for _, script := range fake.stdins {
		expected := "inactive"
		if strings.Contains(script, "check_unit 'app' active") {
			expected = "active"
		}
		for _, unit := range []string{"worker", "mock"} {
			if !strings.Contains(script, "check_unit '"+unit+"' "+expected) {
				t.Fatalf("host omitted %s %s: %s", unit, expected, script)
			}
		}
	}
}
