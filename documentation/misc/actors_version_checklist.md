### Actor version integration checklist

- [ ] Import new actors
- [ ] Define upgrade heights in `build/params_`
- [ ] Generate adapters
  - [ ] Add the new version in `chain/actors/agen/main.go`
  - [ ] Update adapter code in `chain/actors/builtin` if needed
- [ ] Update `chain/actors/policy/policy.go`
- [ ] Update `chain/actors/version.go`
- [ ] Register in `chain/vm/invoker.go`
- [ ] Update `chain/types/state.go`
- [ ] Update `chain/state/statetree.go` (New / Load)
- [ ] Update `chain/stmgr/forks.go`
  - [ ] Schedule
  - [ ] Migration
- [ ] Update upgrade schedule in `chain/sync_test.go`
- [ ] Register in init in `chain/stmgr/utils.go`
