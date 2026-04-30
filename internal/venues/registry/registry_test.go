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

func TestNamesIncludesInitialVenueSet(t *testing.T) {
	names := Names()
	for _, name := range []string{
		"aster",
		"edgex",
		"extended",
		"grvt",
		"hyperliquid",
		"lighter",
		"variational_omni",
	} {
		if !slices.Contains(names, name) {
			t.Fatalf("names = %#v, missing %s", names, name)
		}
	}
}
