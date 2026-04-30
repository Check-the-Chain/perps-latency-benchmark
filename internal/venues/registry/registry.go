package registry

import (
	"fmt"
	"slices"

	"perps-latency-benchmark/internal/names"
	"perps-latency-benchmark/internal/venues/aster"
	"perps-latency-benchmark/internal/venues/edgex"
	"perps-latency-benchmark/internal/venues/extended"
	"perps-latency-benchmark/internal/venues/grvt"
	"perps-latency-benchmark/internal/venues/hyperliquid"
	"perps-latency-benchmark/internal/venues/lighter"
	"perps-latency-benchmark/internal/venues/spec"
	"perps-latency-benchmark/internal/venues/variational_omni"
)

var definitions = []spec.Definition{
	aster.Definition(),
	edgex.Definition(),
	extended.Definition(),
	grvt.Definition(),
	hyperliquid.Definition(),
	lighter.Definition(),
	variational_omni.Definition(),
}

func Lookup(name string) (spec.Definition, bool) {
	target := names.Normalize(name)
	for _, definition := range definitions {
		for _, candidate := range definition.Names() {
			if candidate == target {
				return definition, true
			}
		}
	}
	return spec.Definition{}, false
}

func Names() []string {
	names := make([]string, 0, len(definitions))
	for _, definition := range definitions {
		names = append(names, definition.Name)
	}
	slices.Sort(names)
	return names
}

func MustLookup(name string) (spec.Definition, error) {
	definition, ok := Lookup(name)
	if !ok {
		return spec.Definition{}, fmt.Errorf("unknown venue %q; available venues: %v", name, Names())
	}
	return definition, nil
}
