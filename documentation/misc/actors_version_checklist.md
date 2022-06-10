### Actor version integration checklist (v16 and onwards)

- [ ] Import new go-state-types, if needed.
- [ ] Define upgrade heights in `build/params_`
- [ ] Generate adapters
  - [ ] Update `gen/inlinegen-data.json`
  - [ ] Update `chain/actors/version.go`
  - [ ] Update adapter code in `chain/actors/builtin` if needed
  - [ ] Run `make actors-gen`
- [ ] Update `chain/consensus/filcns/upgrades.go`
  - [ ] Schedule
  - [ ] Migration
    - [ ] Create a new Migration that calls LiteMigration()
- [ ] Update upgrade schedule in `chain/sync_test.go`