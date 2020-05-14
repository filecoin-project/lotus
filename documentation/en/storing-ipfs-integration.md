# IPFS Integration

Lotus supports making deals with data stored in IPFS, without having to re-import it into lotus.

To enable this integration, you need to have an IPFS daemon running in the background.
Then, open up `~/.lotus/config.toml` (or if you manually set `LOTUS_PATH`, look under that directory) 
and look for the Client field, and set `UseIpfs` to `true`.

```toml
[Client]
UseIpfs = true
```

After restarting the lotus daemon, you should be able to make deals with data in your IPFS node:

```sh
$ ipfs add -r SomeData
QmSomeData
$ ./lotus client deal QmSomeData t01000 0.0000000001 80000
```
