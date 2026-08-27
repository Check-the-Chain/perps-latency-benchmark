package registry

import (
	"slices"
	"testing"
)

func TestLookupSupportsKnownVenuesAndAliases(t *testing.T) {
	definition, ok := Lookup("hyperliquid")
	if !ok {
		t.Fatal("expected hyperliquid definition")
	}
	if definition.Name != "hyperliquid" {
		t.Fatalf("name = %s", definition.Name)
	}

	aliasDefinition, ok := Lookup("hyper-liquid")
	if !ok {
		t.Fatal("expected hyperliquid alias definition")
	}
	if aliasDefinition.Name != "hyperliquid" {
		t.Fatalf("alias name = %s", aliasDefinition.Name)
	}
}

func TestLookupBullet(t *testing.T) {
	for _, name := range []string{"bullet", "Bullet", "bullet-xyz", "bulletx"} {
		definition, ok := Lookup(name)
		if !ok {
			t.Fatalf("lookup %q must resolve", name)
		}
		if definition.Name != "bullet" {
			t.Fatalf("lookup %q resolved to %q", name, definition.Name)
		}
	}
}

func TestNamesIncludesInitialVenueSet(t *testing.T) {
	names := Names()
	for _, name := range []string{
		"aster",
		"edgex",
		"extended",
		"grvt",
		"hyperliquid",
		"lighter",
		"lighter_free",
		"nado",
		"nado_direct",
		"pacifica",
		"variational_omni",
	} {
		if !slices.Contains(names, name) {
			t.Fatalf("names = %#v, missing %s", names, name)
		}
	}
}
