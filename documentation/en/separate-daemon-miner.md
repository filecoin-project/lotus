# Running Lotus Daemon and Lotus-Miner on separate hosts

To setup the lotus daemon and lotus-miner on separate hosts, first setup up the daemon on your chosen machine. Make sure that the lotus daemon's API is configured to be accessible by an external host.

```sh
[API]
  ListenAddress = "/ip4/0.0.0.0/tcp/1234/http"
```

Once the lotus daemon is configured and running, the lotus miner host needs to be configured. On the lotus miner machine, create a ~/.lotus directory like on the lotus daemon machine but only create the ~/.lotus/api and ~/.lotus/token files. Make sure that the ~/.lotus/api files contains the appropriate multiaddress for the machine that is running the lotus daemon, i.e.

```sh 
$ cat ~/.lotus/api
/ip4/192.168.1.11/tcp/1234/http
```

Note, this should differ from the contents of the file on the lotus daemon machine.

For the token file, you can copy the contents of the ~/.lotus/token file on the lotus daemon machine exactly.

So on the miner machine you end up with a ~/.lotus directory that looks like the following:

```sh
$ ls ~/.lotus
-rw-r--r-- 1 lotus lotus  32 Aug 25 15:49 api
-rw-r--r-- 1 lotus lotus 137 Aug 25 15:49 token
```

