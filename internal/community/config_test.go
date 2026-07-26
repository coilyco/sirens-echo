package community

import (
	"path/filepath"
	"testing"
)

func TestLoadDefinition(t *testing.T) {
	t.Parallel()
	path := filepath.Join("..", "..", "agent", "sirens-echo.json")
	definition, err := LoadDefinition(path)
	if err != nil {
		t.Fatalf("LoadDefinition: %v", err)
	}
	if definition.Identity != "Sirens Echo" {
		t.Fatalf("identity = %q", definition.Identity)
	}
	if definition.Seat != "siren community host" {
		t.Fatalf("seat = %q", definition.Seat)
	}
	if definition.Role != "community" {
		t.Fatalf("role = %q", definition.Role)
	}
	if definition.Model != "ornith:35b" {
		t.Fatalf("model = %q", definition.Model)
	}
	if definition.Channel != "#bots" {
		t.Fatalf("channel = %q", definition.Channel)
	}
	if len(definition.SkillRoots) != 1 {
		t.Fatalf("skill roots = %#v", definition.SkillRoots)
	}
}
