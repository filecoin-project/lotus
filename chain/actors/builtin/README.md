# Actors

This package contains shims for abstracting over different actor versions.

## Design

Shims in this package follow a few common design principles.

### Structure Agnostic

Shims interfaces defined in this package should (ideally) not change even if the
structure of the underlying data changes. For example:

* All shims store an internal "store" object. That way, state can be moved into
  a separate object without needing to add a store to the function signature.
* All functions must return an error, even if unused for now.

### Minimal

These interfaces should be expanded only as necessary to reduce maintenance burden.

### Queries, not field assessors.

When possible, functions should query the state instead of simply acting as
field assessors. These queries are more likely to remain stable across
specs-actor upgrades than specific state fields.

Note: there is a trade-off here. Avoid implementing _complicated_ query logic
inside these shims, as it will need to be replicated in every shim.
