# Delegator Actor (Scaffold)

Purpose: apply EIP-7702 authorization tuples and route EOA calls via delegate code.

This folder is a scaffold only. It defines parameter types and method names to help
future agents wire the actor/FVM path without blocking the current build.

Suggested steps (see AGENTS.md for details):
- Implement state: mapping `EOA -> DelegateAddr` and optional metadata.
- Implement `ApplyDelegations` method to validate tuples and write mappings.
- Wire EVM call path to check mapping for EOA targets and execute delegate code.
- Add gas charging/refunds per EIP-7702.

