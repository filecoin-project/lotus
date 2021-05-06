### Actor version integration checklist

- [ ] Import new actors
- [ ] Define upgrade heights in `build/params_`
- [ ] Generate adapters
  - [ ] Add the new version in `chain/actors/agen/main.go`
  - [ ] Update adapter code in `chain/actors/builtin` if needed
- [ ] Update `chain/actors/policy/policy.go`
- [ ] Update `chain/actors/version.go`
- [ ] Register in `chain/vm/invoker.go`
- [ ] Register in `chain/vm/mkactor.go`
- [ ] Update `chain/types/state.go`
- [ ] Update `chain/state/statetree.go`
- [ ] Update `chain/stmgr/forks.go`
  - [ ] Schedule
  - [ ] Migration
- [ ] Update upgrade schedule in `api/test/test.go`
- [ ] Update `NewestNetworkVersion` in `build/params_shared_vals.go`
- [ ] Register in init in `chain/stmgr/utils.go`
