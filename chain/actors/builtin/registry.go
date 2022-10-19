package builtin

import (
	"reflect"
	"runtime"
	"strings"

	"github.com/ipfs/go-cid"

	actorstypes "github.com/filecoin-project/go-state-types/actors"
	"github.com/filecoin-project/go-state-types/builtin"
	account8 "github.com/filecoin-project/go-state-types/builtin/v8/account"
	cron8 "github.com/filecoin-project/go-state-types/builtin/v8/cron"
	_init8 "github.com/filecoin-project/go-state-types/builtin/v8/init"
	market8 "github.com/filecoin-project/go-state-types/builtin/v8/market"
	miner8 "github.com/filecoin-project/go-state-types/builtin/v8/miner"
	multisig8 "github.com/filecoin-project/go-state-types/builtin/v8/multisig"
	paych8 "github.com/filecoin-project/go-state-types/builtin/v8/paych"
	power8 "github.com/filecoin-project/go-state-types/builtin/v8/power"
	reward8 "github.com/filecoin-project/go-state-types/builtin/v8/reward"
	system8 "github.com/filecoin-project/go-state-types/builtin/v8/system"
	verifreg8 "github.com/filecoin-project/go-state-types/builtin/v8/verifreg"
	account9 "github.com/filecoin-project/go-state-types/builtin/v9/account"
	cron9 "github.com/filecoin-project/go-state-types/builtin/v9/cron"
	datacap9 "github.com/filecoin-project/go-state-types/builtin/v9/datacap"
	_init9 "github.com/filecoin-project/go-state-types/builtin/v9/init"
	market9 "github.com/filecoin-project/go-state-types/builtin/v9/market"
	miner9 "github.com/filecoin-project/go-state-types/builtin/v9/miner"
	multisig9 "github.com/filecoin-project/go-state-types/builtin/v9/multisig"
	paych9 "github.com/filecoin-project/go-state-types/builtin/v9/paych"
	power9 "github.com/filecoin-project/go-state-types/builtin/v9/power"
	reward9 "github.com/filecoin-project/go-state-types/builtin/v9/reward"
	system9 "github.com/filecoin-project/go-state-types/builtin/v9/system"
	verifreg9 "github.com/filecoin-project/go-state-types/builtin/v9/verifreg"
	"github.com/filecoin-project/go-state-types/cbor"
	rtt "github.com/filecoin-project/go-state-types/rt"

	"github.com/filecoin-project/lotus/chain/actors"
)

type RegistryEntry struct {
	state   cbor.Er
	code    cid.Cid
	methods map[uint64]builtin.MethodMeta
}

func (r RegistryEntry) State() cbor.Er {
	return r.state
}

func (r RegistryEntry) Exports() map[uint64]builtin.MethodMeta {
	return r.methods
}

func (r RegistryEntry) Code() cid.Cid {
	return r.code
}

func MakeRegistryLegacy(actors []rtt.VMActor) []RegistryEntry {
	registry := make([]RegistryEntry, 0)

	for _, actor := range actors {
		methodMap := make(map[uint64]builtin.MethodMeta)
		for methodNum, method := range actor.Exports() {
			if method != nil {
				methodMap[uint64(methodNum)] = makeMethodMeta(method)
			}
		}
		registry = append(registry, RegistryEntry{
			code:    actor.Code(),
			methods: methodMap,
			state:   actor.State(),
		})
	}

	return registry
}

func makeMethodMeta(method interface{}) builtin.MethodMeta {
	ev := reflect.ValueOf(method)
	// Extract the method names using reflection. These
	// method names always match the field names in the
	// `builtin.Method*` structs (tested in the specs-actors
	// tests).
	fnName := runtime.FuncForPC(ev.Pointer()).Name()
	fnName = strings.TrimSuffix(fnName[strings.LastIndexByte(fnName, '.')+1:], "-fm")
	return builtin.MethodMeta{
		Name:   fnName,
		Method: method,
	}
}

func MakeRegistry(av actorstypes.Version) []RegistryEntry {
	if av < actorstypes.Version8 {
		panic("expected version v8 and up only, use specs-actors for v0-7")
	}
	registry := make([]RegistryEntry, 0)

	codeIDs, err := actors.GetActorCodeIDs(av)
	if err != nil {
		panic(err)
	}

	switch av {

	case actorstypes.Version8:
		for key, codeID := range codeIDs {
			switch key {
			case actors.AccountKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: account8.Methods,
					state:   new(account8.State),
				})
			case actors.CronKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: cron8.Methods,
					state:   new(cron8.State),
				})
			case actors.InitKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: _init8.Methods,
					state:   new(_init8.State),
				})
			case actors.MarketKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: market8.Methods,
					state:   new(market8.State),
				})
			case actors.MinerKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: miner8.Methods,
					state:   new(miner8.State),
				})
			case actors.MultisigKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: multisig8.Methods,
					state:   new(multisig8.State),
				})
			case actors.PaychKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: paych8.Methods,
					state:   new(paych8.State),
				})
			case actors.PowerKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: power8.Methods,
					state:   new(power8.State),
				})
			case actors.RewardKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: reward8.Methods,
					state:   new(reward8.State),
				})
			case actors.SystemKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: system8.Methods,
					state:   new(system8.State),
				})
			case actors.VerifregKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: verifreg8.Methods,
					state:   new(verifreg8.State),
				})

			}
		}

	case actorstypes.Version9:
		for key, codeID := range codeIDs {
			switch key {
			case actors.AccountKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: account9.Methods,
					state:   new(account9.State),
				})
			case actors.CronKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: cron9.Methods,
					state:   new(cron9.State),
				})
			case actors.InitKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: _init9.Methods,
					state:   new(_init9.State),
				})
			case actors.MarketKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: market9.Methods,
					state:   new(market9.State),
				})
			case actors.MinerKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: miner9.Methods,
					state:   new(miner9.State),
				})
			case actors.MultisigKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: multisig9.Methods,
					state:   new(multisig9.State),
				})
			case actors.PaychKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: paych9.Methods,
					state:   new(paych9.State),
				})
			case actors.PowerKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: power9.Methods,
					state:   new(power9.State),
				})
			case actors.RewardKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: reward9.Methods,
					state:   new(reward9.State),
				})
			case actors.SystemKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: system9.Methods,
					state:   new(system9.State),
				})
			case actors.VerifregKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: verifreg9.Methods,
					state:   new(verifreg9.State),
				})
			case actors.DatacapKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: datacap9.Methods,
					state:   new(datacap9.State),
				})
			}
		}

	default:
		panic("expected version v8 and up only, use specs-actors for v0-7")
	}

	return registry
}
