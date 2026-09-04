# Copilot code review instructions

Use `AGENTS.md` for Lotus architecture, invariants and validation commands. Review the pull request's changed behaviour, not the repository in the abstract. Inspect relevant callers, interfaces and tests before commenting.

Prioritise, in order:

1. Consensus divergence, invalid state transitions, incorrect network-version gates and errors around tipsets, null rounds, deferred execution or reorgs.
2. Security and denial-of-service problems at RPC, gateway, chain-exchange and other untrusted-input boundaries. Compare the trivial cost of sending a request with its worst-case CPU, memory, I/O, state-replay and chain-walk cost, including concurrent amplification. Flag attacker-controlled ranges, counts, recursion, scans or results that lack a defensible bound.
3. Data loss or corruption, including non-atomic writes, incompatible schemas, stale caches and restart or revert paths that disagree with the forward path.
4. API compatibility and completeness across interfaces, implementations, permission annotations, version wrappers, gateway exposure, generated proxies, mocks and OpenRPC output. New API work should normally target v2; v0/v1 are compatibility surfaces and should change only for an explicit compatibility need or bug fix.
5. Goroutine leaks, deadlocks, races, blocking channel operations, missing cancellation and unbounded queues or retries.
6. Tests that miss the changed contract, generated outputs that no longer match their source and user-visible changes missing the required changelog treatment.

Only leave a comment when there is a concrete, plausible failure introduced by the pull request. State the triggering input or system state, the incorrect result and its consequence. Verify assumptions against the current code; Lotus has intentional patterns that can look unusual in isolation.

Do not report style preferences, speculative hardening, unrelated pre-existing problems or refactors without a behavioural benefit. Do not ask for manual edits to generated files; identify the source or generator that must change. For network-upgrade logic, inspect both sides of the activation boundary. For API changes, trace the full path rather than reviewing one interface or implementation in isolation. Treat `lotus-gateway` as a distinct reverse-proxy boundary: new or changed methods need an appropriate cost class and method-specific lookback or range limits where work scales with history or caller-controlled size; existing global and per-connection rate limits do not make an individually unbounded method safe.
