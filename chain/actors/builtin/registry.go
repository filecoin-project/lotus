package builtin

import (
	"reflect"
	"runtime"
	"strings"

	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/go-state-types/abi"
	actorstypes "github.com/filecoin-project/go-state-types/actors"
	"github.com/filecoin-project/go-state-types/builtin"
	account10 "github.com/filecoin-project/go-state-types/builtin/v10/account"
	cron10 "github.com/filecoin-project/go-state-types/builtin/v10/cron"
	datacap10 "github.com/filecoin-project/go-state-types/builtin/v10/datacap"
	eam10 "github.com/filecoin-project/go-state-types/builtin/v10/eam"
	ethaccount10 "github.com/filecoin-project/go-state-types/builtin/v10/ethaccount"
	evm10 "github.com/filecoin-project/go-state-types/builtin/v10/evm"
	_init10 "github.com/filecoin-project/go-state-types/builtin/v10/init"
	market10 "github.com/filecoin-project/go-state-types/builtin/v10/market"
	miner10 "github.com/filecoin-project/go-state-types/builtin/v10/miner"
	multisig10 "github.com/filecoin-project/go-state-types/builtin/v10/multisig"
	paych10 "github.com/filecoin-project/go-state-types/builtin/v10/paych"
	placeholder10 "github.com/filecoin-project/go-state-types/builtin/v10/placeholder"
	power10 "github.com/filecoin-project/go-state-types/builtin/v10/power"
	reward10 "github.com/filecoin-project/go-state-types/builtin/v10/reward"
	system10 "github.com/filecoin-project/go-state-types/builtin/v10/system"
	verifreg10 "github.com/filecoin-project/go-state-types/builtin/v10/verifreg"
	account11 "github.com/filecoin-project/go-state-types/builtin/v11/account"
	cron11 "github.com/filecoin-project/go-state-types/builtin/v11/cron"
	datacap11 "github.com/filecoin-project/go-state-types/builtin/v11/datacap"
	eam11 "github.com/filecoin-project/go-state-types/builtin/v11/eam"
	ethaccount11 "github.com/filecoin-project/go-state-types/builtin/v11/ethaccount"
	evm11 "github.com/filecoin-project/go-state-types/builtin/v11/evm"
	_init11 "github.com/filecoin-project/go-state-types/builtin/v11/init"
	market11 "github.com/filecoin-project/go-state-types/builtin/v11/market"
	miner11 "github.com/filecoin-project/go-state-types/builtin/v11/miner"
	multisig11 "github.com/filecoin-project/go-state-types/builtin/v11/multisig"
	paych11 "github.com/filecoin-project/go-state-types/builtin/v11/paych"
	placeholder11 "github.com/filecoin-project/go-state-types/builtin/v11/placeholder"
	power11 "github.com/filecoin-project/go-state-types/builtin/v11/power"
	reward11 "github.com/filecoin-project/go-state-types/builtin/v11/reward"
	system11 "github.com/filecoin-project/go-state-types/builtin/v11/system"
	verifreg11 "github.com/filecoin-project/go-state-types/builtin/v11/verifreg"
	account12 "github.com/filecoin-project/go-state-types/builtin/v12/account"
	cron12 "github.com/filecoin-project/go-state-types/builtin/v12/cron"
	datacap12 "github.com/filecoin-project/go-state-types/builtin/v12/datacap"
	eam12 "github.com/filecoin-project/go-state-types/builtin/v12/eam"
	ethaccount12 "github.com/filecoin-project/go-state-types/builtin/v12/ethaccount"
	evm12 "github.com/filecoin-project/go-state-types/builtin/v12/evm"
	_init12 "github.com/filecoin-project/go-state-types/builtin/v12/init"
	market12 "github.com/filecoin-project/go-state-types/builtin/v12/market"
	miner12 "github.com/filecoin-project/go-state-types/builtin/v12/miner"
	multisig12 "github.com/filecoin-project/go-state-types/builtin/v12/multisig"
	paych12 "github.com/filecoin-project/go-state-types/builtin/v12/paych"
	placeholder12 "github.com/filecoin-project/go-state-types/builtin/v12/placeholder"
	power12 "github.com/filecoin-project/go-state-types/builtin/v12/power"
	reward12 "github.com/filecoin-project/go-state-types/builtin/v12/reward"
	system12 "github.com/filecoin-project/go-state-types/builtin/v12/system"
	verifreg12 "github.com/filecoin-project/go-state-types/builtin/v12/verifreg"
	account13 "github.com/filecoin-project/go-state-types/builtin/v13/account"
	cron13 "github.com/filecoin-project/go-state-types/builtin/v13/cron"
	datacap13 "github.com/filecoin-project/go-state-types/builtin/v13/datacap"
	eam13 "github.com/filecoin-project/go-state-types/builtin/v13/eam"
	ethaccount13 "github.com/filecoin-project/go-state-types/builtin/v13/ethaccount"
	evm13 "github.com/filecoin-project/go-state-types/builtin/v13/evm"
	_init13 "github.com/filecoin-project/go-state-types/builtin/v13/init"
	market13 "github.com/filecoin-project/go-state-types/builtin/v13/market"
	miner13 "github.com/filecoin-project/go-state-types/builtin/v13/miner"
	multisig13 "github.com/filecoin-project/go-state-types/builtin/v13/multisig"
	paych13 "github.com/filecoin-project/go-state-types/builtin/v13/paych"
	placeholder13 "github.com/filecoin-project/go-state-types/builtin/v13/placeholder"
	power13 "github.com/filecoin-project/go-state-types/builtin/v13/power"
	reward13 "github.com/filecoin-project/go-state-types/builtin/v13/reward"
	system13 "github.com/filecoin-project/go-state-types/builtin/v13/system"
	verifreg13 "github.com/filecoin-project/go-state-types/builtin/v13/verifreg"
	account14 "github.com/filecoin-project/go-state-types/builtin/v14/account"
	cron14 "github.com/filecoin-project/go-state-types/builtin/v14/cron"
	datacap14 "github.com/filecoin-project/go-state-types/builtin/v14/datacap"
	eam14 "github.com/filecoin-project/go-state-types/builtin/v14/eam"
	ethaccount14 "github.com/filecoin-project/go-state-types/builtin/v14/ethaccount"
	evm14 "github.com/filecoin-project/go-state-types/builtin/v14/evm"
	_init14 "github.com/filecoin-project/go-state-types/builtin/v14/init"
	market14 "github.com/filecoin-project/go-state-types/builtin/v14/market"
	miner14 "github.com/filecoin-project/go-state-types/builtin/v14/miner"
	multisig14 "github.com/filecoin-project/go-state-types/builtin/v14/multisig"
	paych14 "github.com/filecoin-project/go-state-types/builtin/v14/paych"
	placeholder14 "github.com/filecoin-project/go-state-types/builtin/v14/placeholder"
	power14 "github.com/filecoin-project/go-state-types/builtin/v14/power"
	reward14 "github.com/filecoin-project/go-state-types/builtin/v14/reward"
	system14 "github.com/filecoin-project/go-state-types/builtin/v14/system"
	verifreg14 "github.com/filecoin-project/go-state-types/builtin/v14/verifreg"
	account15 "github.com/filecoin-project/go-state-types/builtin/v15/account"
	cron15 "github.com/filecoin-project/go-state-types/builtin/v15/cron"
	datacap15 "github.com/filecoin-project/go-state-types/builtin/v15/datacap"
	eam15 "github.com/filecoin-project/go-state-types/builtin/v15/eam"
	ethaccount15 "github.com/filecoin-project/go-state-types/builtin/v15/ethaccount"
	evm15 "github.com/filecoin-project/go-state-types/builtin/v15/evm"
	_init15 "github.com/filecoin-project/go-state-types/builtin/v15/init"
	market15 "github.com/filecoin-project/go-state-types/builtin/v15/market"
	miner15 "github.com/filecoin-project/go-state-types/builtin/v15/miner"
	multisig15 "github.com/filecoin-project/go-state-types/builtin/v15/multisig"
	paych15 "github.com/filecoin-project/go-state-types/builtin/v15/paych"
	placeholder15 "github.com/filecoin-project/go-state-types/builtin/v15/placeholder"
	power15 "github.com/filecoin-project/go-state-types/builtin/v15/power"
	reward15 "github.com/filecoin-project/go-state-types/builtin/v15/reward"
	system15 "github.com/filecoin-project/go-state-types/builtin/v15/system"
	verifreg15 "github.com/filecoin-project/go-state-types/builtin/v15/verifreg"
	account16 "github.com/filecoin-project/go-state-types/builtin/v16/account"
	cron16 "github.com/filecoin-project/go-state-types/builtin/v16/cron"
	datacap16 "github.com/filecoin-project/go-state-types/builtin/v16/datacap"
	eam16 "github.com/filecoin-project/go-state-types/builtin/v16/eam"
	ethaccount16 "github.com/filecoin-project/go-state-types/builtin/v16/ethaccount"
	evm16 "github.com/filecoin-project/go-state-types/builtin/v16/evm"
	_init16 "github.com/filecoin-project/go-state-types/builtin/v16/init"
	market16 "github.com/filecoin-project/go-state-types/builtin/v16/market"
	miner16 "github.com/filecoin-project/go-state-types/builtin/v16/miner"
	multisig16 "github.com/filecoin-project/go-state-types/builtin/v16/multisig"
	paych16 "github.com/filecoin-project/go-state-types/builtin/v16/paych"
	placeholder16 "github.com/filecoin-project/go-state-types/builtin/v16/placeholder"
	power16 "github.com/filecoin-project/go-state-types/builtin/v16/power"
	reward16 "github.com/filecoin-project/go-state-types/builtin/v16/reward"
	system16 "github.com/filecoin-project/go-state-types/builtin/v16/system"
	verifreg16 "github.com/filecoin-project/go-state-types/builtin/v16/verifreg"
	account17 "github.com/filecoin-project/go-state-types/builtin/v17/account"
	cron17 "github.com/filecoin-project/go-state-types/builtin/v17/cron"
	datacap17 "github.com/filecoin-project/go-state-types/builtin/v17/datacap"
	eam17 "github.com/filecoin-project/go-state-types/builtin/v17/eam"
	ethaccount17 "github.com/filecoin-project/go-state-types/builtin/v17/ethaccount"
	evm17 "github.com/filecoin-project/go-state-types/builtin/v17/evm"
	_init17 "github.com/filecoin-project/go-state-types/builtin/v17/init"
	market17 "github.com/filecoin-project/go-state-types/builtin/v17/market"
	miner17 "github.com/filecoin-project/go-state-types/builtin/v17/miner"
	multisig17 "github.com/filecoin-project/go-state-types/builtin/v17/multisig"
	paych17 "github.com/filecoin-project/go-state-types/builtin/v17/paych"
	placeholder17 "github.com/filecoin-project/go-state-types/builtin/v17/placeholder"
	power17 "github.com/filecoin-project/go-state-types/builtin/v17/power"
	reward17 "github.com/filecoin-project/go-state-types/builtin/v17/reward"
	system17 "github.com/filecoin-project/go-state-types/builtin/v17/system"
	verifreg17 "github.com/filecoin-project/go-state-types/builtin/v17/verifreg"
	account18 "github.com/filecoin-project/go-state-types/builtin/v18/account"
	cron18 "github.com/filecoin-project/go-state-types/builtin/v18/cron"
	datacap18 "github.com/filecoin-project/go-state-types/builtin/v18/datacap"
	eam18 "github.com/filecoin-project/go-state-types/builtin/v18/eam"
	ethaccount18 "github.com/filecoin-project/go-state-types/builtin/v18/ethaccount"
	evm18 "github.com/filecoin-project/go-state-types/builtin/v18/evm"
	_init18 "github.com/filecoin-project/go-state-types/builtin/v18/init"
	market18 "github.com/filecoin-project/go-state-types/builtin/v18/market"
	miner18 "github.com/filecoin-project/go-state-types/builtin/v18/miner"
	multisig18 "github.com/filecoin-project/go-state-types/builtin/v18/multisig"
	paych18 "github.com/filecoin-project/go-state-types/builtin/v18/paych"
	placeholder18 "github.com/filecoin-project/go-state-types/builtin/v18/placeholder"
	power18 "github.com/filecoin-project/go-state-types/builtin/v18/power"
	reward18 "github.com/filecoin-project/go-state-types/builtin/v18/reward"
	system18 "github.com/filecoin-project/go-state-types/builtin/v18/system"
	verifreg18 "github.com/filecoin-project/go-state-types/builtin/v18/verifreg"
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
	"github.com/filecoin-project/go-state-types/manifest"
	rtt "github.com/filecoin-project/go-state-types/rt"

	"github.com/filecoin-project/lotus/chain/actors"
)

type RegistryEntry struct {
	state   cbor.Er
	code    cid.Cid
	methods map[abi.MethodNum]builtin.MethodMeta
}

func (r RegistryEntry) State() cbor.Er {
	return r.state
}

func (r RegistryEntry) Exports() map[abi.MethodNum]builtin.MethodMeta {
	return r.methods
}

func (r RegistryEntry) Code() cid.Cid {
	return r.code
}

func MakeRegistryLegacy(actors []rtt.VMActor) []RegistryEntry {
	registry := make([]RegistryEntry, 0)

	for _, actor := range actors {
		methodMap := make(map[abi.MethodNum]builtin.MethodMeta)
		for methodNum, method := range actor.Exports() {
			if method != nil {
				methodMap[abi.MethodNum(methodNum)] = makeMethodMeta(method)
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
			case manifest.AccountKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: account8.Methods,
					state:   new(account8.State),
				})
			case manifest.CronKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: cron8.Methods,
					state:   new(cron8.State),
				})
			case manifest.InitKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: _init8.Methods,
					state:   new(_init8.State),
				})
			case manifest.MarketKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: market8.Methods,
					state:   new(market8.State),
				})
			case manifest.MinerKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: miner8.Methods,
					state:   new(miner8.State),
				})
			case manifest.MultisigKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: multisig8.Methods,
					state:   new(multisig8.State),
				})
			case manifest.PaychKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: paych8.Methods,
					state:   new(paych8.State),
				})
			case manifest.PowerKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: power8.Methods,
					state:   new(power8.State),
				})
			case manifest.RewardKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: reward8.Methods,
					state:   new(reward8.State),
				})
			case manifest.SystemKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: system8.Methods,
					state:   new(system8.State),
				})
			case manifest.VerifregKey:
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
			case manifest.AccountKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: account9.Methods,
					state:   new(account9.State),
				})
			case manifest.CronKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: cron9.Methods,
					state:   new(cron9.State),
				})
			case manifest.InitKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: _init9.Methods,
					state:   new(_init9.State),
				})
			case manifest.MarketKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: market9.Methods,
					state:   new(market9.State),
				})
			case manifest.MinerKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: miner9.Methods,
					state:   new(miner9.State),
				})
			case manifest.MultisigKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: multisig9.Methods,
					state:   new(multisig9.State),
				})
			case manifest.PaychKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: paych9.Methods,
					state:   new(paych9.State),
				})
			case manifest.PowerKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: power9.Methods,
					state:   new(power9.State),
				})
			case manifest.RewardKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: reward9.Methods,
					state:   new(reward9.State),
				})
			case manifest.SystemKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: system9.Methods,
					state:   new(system9.State),
				})
			case manifest.VerifregKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: verifreg9.Methods,
					state:   new(verifreg9.State),
				})
			case manifest.DatacapKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: datacap9.Methods,
					state:   new(datacap9.State),
				})

			}
		}

	case actorstypes.Version10:
		for key, codeID := range codeIDs {
			switch key {
			case manifest.AccountKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: account10.Methods,
					state:   new(account10.State),
				})
			case manifest.CronKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: cron10.Methods,
					state:   new(cron10.State),
				})
			case manifest.InitKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: _init10.Methods,
					state:   new(_init10.State),
				})
			case manifest.MarketKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: market10.Methods,
					state:   new(market10.State),
				})
			case manifest.MinerKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: miner10.Methods,
					state:   new(miner10.State),
				})
			case manifest.MultisigKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: multisig10.Methods,
					state:   new(multisig10.State),
				})
			case manifest.PaychKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: paych10.Methods,
					state:   new(paych10.State),
				})
			case manifest.PowerKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: power10.Methods,
					state:   new(power10.State),
				})
			case manifest.RewardKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: reward10.Methods,
					state:   new(reward10.State),
				})
			case manifest.SystemKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: system10.Methods,
					state:   new(system10.State),
				})
			case manifest.VerifregKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: verifreg10.Methods,
					state:   new(verifreg10.State),
				})
			case manifest.DatacapKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: datacap10.Methods,
					state:   new(datacap10.State),
				})

			case manifest.EvmKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: evm10.Methods,
					state:   new(evm10.State),
				})
			case manifest.EamKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: eam10.Methods,
					state:   nil,
				})
			case manifest.PlaceholderKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: placeholder10.Methods,
					state:   nil,
				})
			case manifest.EthAccountKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: ethaccount10.Methods,
					state:   nil,
				})

			}
		}

	case actorstypes.Version11:
		for key, codeID := range codeIDs {
			switch key {
			case manifest.AccountKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: account11.Methods,
					state:   new(account11.State),
				})
			case manifest.CronKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: cron11.Methods,
					state:   new(cron11.State),
				})
			case manifest.InitKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: _init11.Methods,
					state:   new(_init11.State),
				})
			case manifest.MarketKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: market11.Methods,
					state:   new(market11.State),
				})
			case manifest.MinerKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: miner11.Methods,
					state:   new(miner11.State),
				})
			case manifest.MultisigKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: multisig11.Methods,
					state:   new(multisig11.State),
				})
			case manifest.PaychKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: paych11.Methods,
					state:   new(paych11.State),
				})
			case manifest.PowerKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: power11.Methods,
					state:   new(power11.State),
				})
			case manifest.RewardKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: reward11.Methods,
					state:   new(reward11.State),
				})
			case manifest.SystemKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: system11.Methods,
					state:   new(system11.State),
				})
			case manifest.VerifregKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: verifreg11.Methods,
					state:   new(verifreg11.State),
				})
			case manifest.DatacapKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: datacap11.Methods,
					state:   new(datacap11.State),
				})

			case manifest.EvmKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: evm11.Methods,
					state:   new(evm11.State),
				})
			case manifest.EamKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: eam11.Methods,
					state:   nil,
				})
			case manifest.PlaceholderKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: placeholder11.Methods,
					state:   nil,
				})
			case manifest.EthAccountKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: ethaccount11.Methods,
					state:   nil,
				})

			}
		}

	case actorstypes.Version12:
		for key, codeID := range codeIDs {
			switch key {
			case manifest.AccountKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: account12.Methods,
					state:   new(account12.State),
				})
			case manifest.CronKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: cron12.Methods,
					state:   new(cron12.State),
				})
			case manifest.InitKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: _init12.Methods,
					state:   new(_init12.State),
				})
			case manifest.MarketKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: market12.Methods,
					state:   new(market12.State),
				})
			case manifest.MinerKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: miner12.Methods,
					state:   new(miner12.State),
				})
			case manifest.MultisigKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: multisig12.Methods,
					state:   new(multisig12.State),
				})
			case manifest.PaychKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: paych12.Methods,
					state:   new(paych12.State),
				})
			case manifest.PowerKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: power12.Methods,
					state:   new(power12.State),
				})
			case manifest.RewardKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: reward12.Methods,
					state:   new(reward12.State),
				})
			case manifest.SystemKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: system12.Methods,
					state:   new(system12.State),
				})
			case manifest.VerifregKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: verifreg12.Methods,
					state:   new(verifreg12.State),
				})
			case manifest.DatacapKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: datacap12.Methods,
					state:   new(datacap12.State),
				})

			case manifest.EvmKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: evm12.Methods,
					state:   new(evm12.State),
				})
			case manifest.EamKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: eam12.Methods,
					state:   nil,
				})
			case manifest.PlaceholderKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: placeholder12.Methods,
					state:   nil,
				})
			case manifest.EthAccountKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: ethaccount12.Methods,
					state:   nil,
				})

			}
		}

	case actorstypes.Version13:
		for key, codeID := range codeIDs {
			switch key {
			case manifest.AccountKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: account13.Methods,
					state:   new(account13.State),
				})
			case manifest.CronKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: cron13.Methods,
					state:   new(cron13.State),
				})
			case manifest.InitKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: _init13.Methods,
					state:   new(_init13.State),
				})
			case manifest.MarketKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: market13.Methods,
					state:   new(market13.State),
				})
			case manifest.MinerKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: miner13.Methods,
					state:   new(miner13.State),
				})
			case manifest.MultisigKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: multisig13.Methods,
					state:   new(multisig13.State),
				})
			case manifest.PaychKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: paych13.Methods,
					state:   new(paych13.State),
				})
			case manifest.PowerKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: power13.Methods,
					state:   new(power13.State),
				})
			case manifest.RewardKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: reward13.Methods,
					state:   new(reward13.State),
				})
			case manifest.SystemKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: system13.Methods,
					state:   new(system13.State),
				})
			case manifest.VerifregKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: verifreg13.Methods,
					state:   new(verifreg13.State),
				})
			case manifest.DatacapKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: datacap13.Methods,
					state:   new(datacap13.State),
				})

			case manifest.EvmKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: evm13.Methods,
					state:   new(evm13.State),
				})
			case manifest.EamKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: eam13.Methods,
					state:   nil,
				})
			case manifest.PlaceholderKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: placeholder13.Methods,
					state:   nil,
				})
			case manifest.EthAccountKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: ethaccount13.Methods,
					state:   nil,
				})

			}
		}

	case actorstypes.Version14:
		for key, codeID := range codeIDs {
			switch key {
			case manifest.AccountKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: account14.Methods,
					state:   new(account14.State),
				})
			case manifest.CronKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: cron14.Methods,
					state:   new(cron14.State),
				})
			case manifest.InitKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: _init14.Methods,
					state:   new(_init14.State),
				})
			case manifest.MarketKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: market14.Methods,
					state:   new(market14.State),
				})
			case manifest.MinerKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: miner14.Methods,
					state:   new(miner14.State),
				})
			case manifest.MultisigKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: multisig14.Methods,
					state:   new(multisig14.State),
				})
			case manifest.PaychKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: paych14.Methods,
					state:   new(paych14.State),
				})
			case manifest.PowerKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: power14.Methods,
					state:   new(power14.State),
				})
			case manifest.RewardKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: reward14.Methods,
					state:   new(reward14.State),
				})
			case manifest.SystemKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: system14.Methods,
					state:   new(system14.State),
				})
			case manifest.VerifregKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: verifreg14.Methods,
					state:   new(verifreg14.State),
				})
			case manifest.DatacapKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: datacap14.Methods,
					state:   new(datacap14.State),
				})

			case manifest.EvmKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: evm14.Methods,
					state:   new(evm14.State),
				})
			case manifest.EamKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: eam14.Methods,
					state:   nil,
				})
			case manifest.PlaceholderKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: placeholder14.Methods,
					state:   nil,
				})
			case manifest.EthAccountKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: ethaccount14.Methods,
					state:   nil,
				})

			}
		}

	case actorstypes.Version15:
		for key, codeID := range codeIDs {
			switch key {
			case manifest.AccountKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: account15.Methods,
					state:   new(account15.State),
				})
			case manifest.CronKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: cron15.Methods,
					state:   new(cron15.State),
				})
			case manifest.InitKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: _init15.Methods,
					state:   new(_init15.State),
				})
			case manifest.MarketKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: market15.Methods,
					state:   new(market15.State),
				})
			case manifest.MinerKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: miner15.Methods,
					state:   new(miner15.State),
				})
			case manifest.MultisigKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: multisig15.Methods,
					state:   new(multisig15.State),
				})
			case manifest.PaychKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: paych15.Methods,
					state:   new(paych15.State),
				})
			case manifest.PowerKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: power15.Methods,
					state:   new(power15.State),
				})
			case manifest.RewardKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: reward15.Methods,
					state:   new(reward15.State),
				})
			case manifest.SystemKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: system15.Methods,
					state:   new(system15.State),
				})
			case manifest.VerifregKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: verifreg15.Methods,
					state:   new(verifreg15.State),
				})
			case manifest.DatacapKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: datacap15.Methods,
					state:   new(datacap15.State),
				})

			case manifest.EvmKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: evm15.Methods,
					state:   new(evm15.State),
				})
			case manifest.EamKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: eam15.Methods,
					state:   nil,
				})
			case manifest.PlaceholderKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: placeholder15.Methods,
					state:   nil,
				})
			case manifest.EthAccountKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: ethaccount15.Methods,
					state:   nil,
				})

			}
		}

	case actorstypes.Version16:
		for key, codeID := range codeIDs {
			switch key {
			case manifest.AccountKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: account16.Methods,
					state:   new(account16.State),
				})
			case manifest.CronKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: cron16.Methods,
					state:   new(cron16.State),
				})
			case manifest.InitKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: _init16.Methods,
					state:   new(_init16.State),
				})
			case manifest.MarketKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: market16.Methods,
					state:   new(market16.State),
				})
			case manifest.MinerKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: miner16.Methods,
					state:   new(miner16.State),
				})
			case manifest.MultisigKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: multisig16.Methods,
					state:   new(multisig16.State),
				})
			case manifest.PaychKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: paych16.Methods,
					state:   new(paych16.State),
				})
			case manifest.PowerKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: power16.Methods,
					state:   new(power16.State),
				})
			case manifest.RewardKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: reward16.Methods,
					state:   new(reward16.State),
				})
			case manifest.SystemKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: system16.Methods,
					state:   new(system16.State),
				})
			case manifest.VerifregKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: verifreg16.Methods,
					state:   new(verifreg16.State),
				})
			case manifest.DatacapKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: datacap16.Methods,
					state:   new(datacap16.State),
				})

			case manifest.EvmKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: evm16.Methods,
					state:   new(evm16.State),
				})
			case manifest.EamKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: eam16.Methods,
					state:   nil,
				})
			case manifest.PlaceholderKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: placeholder16.Methods,
					state:   nil,
				})
			case manifest.EthAccountKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: ethaccount16.Methods,
					state:   nil,
				})

			}
		}

	case actorstypes.Version17:
		for key, codeID := range codeIDs {
			switch key {
			case manifest.AccountKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: account17.Methods,
					state:   new(account17.State),
				})
			case manifest.CronKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: cron17.Methods,
					state:   new(cron17.State),
				})
			case manifest.InitKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: _init17.Methods,
					state:   new(_init17.State),
				})
			case manifest.MarketKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: market17.Methods,
					state:   new(market17.State),
				})
			case manifest.MinerKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: miner17.Methods,
					state:   new(miner17.State),
				})
			case manifest.MultisigKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: multisig17.Methods,
					state:   new(multisig17.State),
				})
			case manifest.PaychKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: paych17.Methods,
					state:   new(paych17.State),
				})
			case manifest.PowerKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: power17.Methods,
					state:   new(power17.State),
				})
			case manifest.RewardKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: reward17.Methods,
					state:   new(reward17.State),
				})
			case manifest.SystemKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: system17.Methods,
					state:   new(system17.State),
				})
			case manifest.VerifregKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: verifreg17.Methods,
					state:   new(verifreg17.State),
				})
			case manifest.DatacapKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: datacap17.Methods,
					state:   new(datacap17.State),
				})

			case manifest.EvmKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: evm17.Methods,
					state:   new(evm17.State),
				})
			case manifest.EamKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: eam17.Methods,
					state:   nil,
				})
			case manifest.PlaceholderKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: placeholder17.Methods,
					state:   nil,
				})
			case manifest.EthAccountKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: ethaccount17.Methods,
					state:   nil,
				})

			}
		}

	case actorstypes.Version18:
		for key, codeID := range codeIDs {
			switch key {
			case manifest.AccountKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: account18.Methods,
					state:   new(account18.State),
				})
			case manifest.CronKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: cron18.Methods,
					state:   new(cron18.State),
				})
			case manifest.InitKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: _init18.Methods,
					state:   new(_init18.State),
				})
			case manifest.MarketKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: market18.Methods,
					state:   new(market18.State),
				})
			case manifest.MinerKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: miner18.Methods,
					state:   new(miner18.State),
				})
			case manifest.MultisigKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: multisig18.Methods,
					state:   new(multisig18.State),
				})
			case manifest.PaychKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: paych18.Methods,
					state:   new(paych18.State),
				})
			case manifest.PowerKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: power18.Methods,
					state:   new(power18.State),
				})
			case manifest.RewardKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: reward18.Methods,
					state:   new(reward18.State),
				})
			case manifest.SystemKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: system18.Methods,
					state:   new(system18.State),
				})
			case manifest.VerifregKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: verifreg18.Methods,
					state:   new(verifreg18.State),
				})
			case manifest.DatacapKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: datacap18.Methods,
					state:   new(datacap18.State),
				})

			case manifest.EvmKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: evm18.Methods,
					state:   new(evm18.State),
				})
			case manifest.EamKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: eam18.Methods,
					state:   nil,
				})
			case manifest.PlaceholderKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: placeholder18.Methods,
					state:   nil,
				})
			case manifest.EthAccountKey:
				registry = append(registry, RegistryEntry{
					code:    codeID,
					methods: ethaccount18.Methods,
					state:   nil,
				})

			}
		}

	default:
		panic("expected version v8 and up only, use specs-actors for v0-7")
	}

	return registry
}
