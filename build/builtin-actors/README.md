This directory contains the builtin actors bundles to be emdedded in the binary.

The base requirement is to have a v8 actors bundle, which lives in builtin-actors-v8.car.
By default, this is embedded and loaded at startup.

You can also specify a custom v7 bundle for testing, by putting your bundle in builtin-actors-v7.car.
If you don't supply one, the fvm builtin will be used.

To force use of a custom bundle by the FVM, you need to set LOTUS_USE_FVM_CUSTOM_BUNDLE=1.
