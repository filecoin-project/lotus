### Actor version integration checklist

- [ ] Import new actors
- [ ] Define upgrade heights in `build/params_`
- [ ] Generate adapters
  - [ ] Update `gen/inlinegen-data.json`
  - [ ] Update adapter code in `chain/actors/version.go` if needed
  - [ ] Update adapter code in `chain/actors/builtin` if needed
  - [ ] Run `make actors-gen`
- [ ] Update `chain/consensus/filcns/upgrades.go`
  - [ ] Schedule
  - [ ] Migration
- [ ] Update upgrade schedule in `chain/sync_test.go`
