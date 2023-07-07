### Actor version integration checklist

- [ ] Import new actors
- [ ] Define upgrade heights in `build/params_`
- [ ] Generate adapters
  - [ ] Update `gen/inlinegen-data.json`
  - [ ] Update adapter code in `chain/actors/builtin` if needed
  - [ ] Run `make actors-gen`
- [ ] Update `chain/consensus/filcns/upgrades.go`
  - [ ] Schedule
  - [ ] Migration
- [ ] Add upgrade field to `api/types.go/ForkUpgradeParams` and update the implementation of StateGetNetworkParams
