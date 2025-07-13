package builtin_test

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
	cbg "github.com/whyrusleeping/cbor-gen"

	actorstypes "github.com/filecoin-project/go-state-types/actors"

	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/builtin"
	"github.com/filecoin-project/lotus/chain/vm"
)

// TestAllActorTypesCBORMarshaling checks that all types in the actor registry
// implement the CBORMarshaler and CBORUnmarshaler interfaces
func TestAllActorTypesCBORMarshaling(t *testing.T) {
	allTypesMissingCBOR := []string{}

	for _, v := range actors.Versions {
		if v < 8 { // Skip versions before 8, as they's not referenced in the registry
			continue
		}
		av := actorstypes.Version(v)

		t.Logf("\n=== Checking Actor Version %d ===", av)

		// Create an ActorRegistry like the VM does
		ar := vm.NewActorRegistry()

		// Register actors
		vmActors := builtin.MakeRegistry(av)
		ar.Register(av, nil, vmActors)

		// Get actor code IDs for naming
		codeIDs, err := actors.GetActorCodeIDs(av)
		require.NoError(t, err, "Failed to get code IDs for version %d", av)

		// Create reverse mapping from code ID to actor name
		codeToName := make(map[string]string)
		for name, codeID := range codeIDs {
			codeToName[codeID.String()] = name
		}

		// Track statistics for this version
		totalChecked := 0
		typesMissingCBOR := []string{}

		// Check each actor's methods
		for codeID, methods := range ar.Methods {
			actorName := codeToName[codeID.String()]
			if actorName == "" {
				actorName = fmt.Sprintf("Unknown(%s)", codeID)
			}

			for methodNum, methodMeta := range methods {
				// Check params type
				if methodMeta.Params != nil && methodMeta.Params.Kind() != reflect.Invalid {
					totalChecked++
					if !implementsCBORInterfaces(methodMeta.Params) {
						msg := fmt.Sprintf("v%d %s Method %d (%s) - Params type %s",
							av, actorName, methodNum, methodMeta.Name, methodMeta.Params)
						typesMissingCBOR = append(typesMissingCBOR, msg)
						allTypesMissingCBOR = append(allTypesMissingCBOR, msg)
					}
				}

				// Check return type
				if methodMeta.Ret != nil && methodMeta.Ret.Kind() != reflect.Invalid {
					totalChecked++
					if !implementsCBORInterfaces(methodMeta.Ret) {
						msg := fmt.Sprintf("v%d %s Method %d (%s) - Return type %s",
							av, actorName, methodNum, methodMeta.Name, methodMeta.Ret)
						typesMissingCBOR = append(typesMissingCBOR, msg)
						allTypesMissingCBOR = append(allTypesMissingCBOR, msg)
					}
				}
			}
		}

		// Also check state types from the registry
		for _, entry := range vmActors {
			codeStr := entry.Code().String()
			actorName := codeToName[codeStr]
			if actorName == "" {
				actorName = fmt.Sprintf("Unknown(%s)", entry.Code())
			}

			if entry.State() != nil {
				totalChecked++
				stateType := reflect.TypeOf(entry.State())
				if !implementsCBORInterfaces(stateType) {
					msg := fmt.Sprintf("v%d %s - State type %s",
						av, actorName, stateType)
					typesMissingCBOR = append(typesMissingCBOR, msg)
					allTypesMissingCBOR = append(allTypesMissingCBOR, msg)
				}
			}
		}

		t.Logf("Version %d: Checked %d types, %d missing CBOR marshaling",
			av, totalChecked, len(typesMissingCBOR))

		if len(typesMissingCBOR) > 0 {
			t.Logf("Types missing CBOR in v%d:", av)
			for _, typeInfo := range typesMissingCBOR {
				t.Logf("  ❌ %s", typeInfo)
			}
		}
	}

	t.Logf("\n=== FINAL SUMMARY ===")
	t.Logf("Total types missing CBOR marshaling across all versions: %d", len(allTypesMissingCBOR))

	if len(allTypesMissingCBOR) > 0 {
		// Print unique types for easier analysis
		uniqueTypes := make(map[string]bool)
		for _, msg := range allTypesMissingCBOR {
			uniqueTypes[msg] = true
		}

		t.Logf("\nUnique types missing CBOR marshaling:")
		for typeInfo := range uniqueTypes {
			t.Logf("  - %s", typeInfo)
		}

		// This test is expected to fail currently
		require.Failf(t, "CBOR marshaling missing", "Found %d types that do not implement CBORMarshaler/CBORUnmarshaler interfaces", len(uniqueTypes))
	}
}

// implementsCBORInterfaces checks if a reflect.Type implements both CBORMarshaler and CBORUnmarshaler
func implementsCBORInterfaces(t reflect.Type) bool {
	if t == nil || t.Kind() == reflect.Invalid {
		return true // nil/invalid is considered "implementing" for our purposes
	}

	// Get interface types
	marshalerType := reflect.TypeOf((*cbg.CBORMarshaler)(nil)).Elem()
	unmarshalerType := reflect.TypeOf((*cbg.CBORUnmarshaler)(nil)).Elem()

	// For interface types, we can't check implementation
	if t.Kind() == reflect.Interface {
		return true // assume interfaces are OK
	}

	// Check if either the type or pointer to type implements both interfaces
	implementsAsValue := t.Implements(marshalerType) && t.Implements(unmarshalerType)

	// For non-pointer types, also check if pointer to type implements
	if t.Kind() != reflect.Ptr && t.Kind() != reflect.Interface {
		ptrType := reflect.PointerTo(t)
		implementsAsPointer := ptrType.Implements(marshalerType) && ptrType.Implements(unmarshalerType)
		return implementsAsValue || implementsAsPointer
	}

	return implementsAsValue
}
