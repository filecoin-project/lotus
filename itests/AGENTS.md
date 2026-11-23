# Integration tests (itests)

The root `AGENTS.md` covers file scoping and where new tests go.

- A test belongs in the file whose class of behaviour it exercises. Among files that fit, job times break the tie: padding a short job is fine; lengthening the longest jobs slows every PR. Create a new file only when nothing fits and the test is long enough to warrant its own job.
- Renaming a file so its name describes its class is fine. `cmd/ci/main.go` keys runner and expensive-test entries on `itest-<file>`, so carry those over to the new name.
- The latest green master run gives each file's minutes, longest first (each currently includes about 2 to 3 minutes of setup):

  ```sh
  id=$(gh run list -R filecoin-project/lotus -w test.yml -b master -s success -L 1 --json databaseId -q '.[0].databaseId')
  gh run view -R filecoin-project/lotus "$id" --json jobs \
    -q '.jobs[] | select(.name | test("itest-")) | [(((.completedAt | fromdate) - (.startedAt | fromdate)) / 60 | floor), .name] | @tsv' | sort -rn
  ```

- CI discovers `itests/*_test.go` files itself (`cmd/ci/main.go`) and runs each on the default runner unless `getRunners` names a bigger one. A heavy new test, in a new or existing file, may need an override there.
- `conformance/` doesn't exercise the FVM: its vectors only have variants at nv0 to nv5 (chaos vectors always use the legacy VM). FVM vectors run in ref-fvm's `testing/conformance`. For FVM coverage inside Lotus use itests; `migration_test.go` has a test per upgrade from nv17 on (`TestMigrationNV17` upgrades nv16 to nv17), each in its own ensemble, which between them cover fvm2, fvm3 and fvm4.
- A miner whose last block attempt races shutdown can log `fatal error (10)` with `cgo blockstore 'get' failed with error code -3` after "shutting down mining", reading a closed blockstore after the test has reported PASS. Treat it as teardown only when it follows that sequence.
- To test an unreleased builtin-actors build, see `documentation/misc/Builtin-actors_Development.md` (`LOTUS_BUILTIN_ACTORS_V<N>_BUNDLE`, `build/actors/pack.sh`, `kit.UpgradeSchedule`).
