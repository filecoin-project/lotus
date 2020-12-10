# Delving into the unknown

This write-up summarises how to debug what appears to be a mischievous Lotus
instance during our Testground tests. It also goes enumerates which assets are
useful to report suspicious behaviours upstream, in a way that they are
actionable.

## Querying the Lotus RPC API

The `local:docker` and `cluster:k8s` map ports that you specify in the
composition.toml, so you can access them externally.

All our compositions should carry this fragment:

```toml
[global.run_config]
  exposed_ports = { pprof = "6060", node_rpc = "1234", miner_rpc = "2345" }
```

This tells Testground to expose the following ports:

* `6060` => Go pprof.
* `1234` => Lotus full node RPC.
* `2345` => Lotus storage miner RPC.

### `local:docker`

1. Install the `lotus` binary on your host.
2. Find the container that you want to connect to in `docker ps`.
     * Note that our _container names_ are slightly long, and they're the last
       field on every line, so if your terminal is wrapping text, the port
       numbers will end up ABOVE the friendly/recognizable container name (e.g. `tg-lotus-soup-deals-e2e-acfc60bc1727-miners-1`).
     * The testground output displays the _container ID_ inside coloured angle
       brackets, so if you spot something spurious in a particular node, you can
       hone in on that one, e.g. `<< 54dd5ad916b2 >>`.

    ```
    ⟩ docker ps
    CONTAINER ID        IMAGE                       COMMAND                  CREATED             STATUS              PORTS                                                                                                NAMES
    54dd5ad916b2        be3c18d7f0d4                "/testplan"              10 seconds ago      Up 8 seconds        0.0.0.0:32788->1234/tcp, 0.0.0.0:32783->2345/tcp, 0.0.0.0:32773->6060/tcp, 0.0.0.0:32777->6060/tcp   tg-lotus-soup-deals-e2e-acfc60bc1727-clients-2
    53757489ce71        be3c18d7f0d4                "/testplan"              10 seconds ago      Up 8 seconds        0.0.0.0:32792->1234/tcp, 0.0.0.0:32790->2345/tcp, 0.0.0.0:32781->6060/tcp, 0.0.0.0:32786->6060/tcp   tg-lotus-soup-deals-e2e-acfc60bc1727-clients-1
    9d3e83b71087        be3c18d7f0d4                "/testplan"              10 seconds ago      Up 8 seconds        0.0.0.0:32791->1234/tcp, 0.0.0.0:32789->2345/tcp, 0.0.0.0:32779->6060/tcp, 0.0.0.0:32784->6060/tcp   tg-lotus-soup-deals-e2e-acfc60bc1727-clients-0
    7bd60e75ed0e        be3c18d7f0d4                "/testplan"              10 seconds ago      Up 8 seconds        0.0.0.0:32787->1234/tcp, 0.0.0.0:32782->2345/tcp, 0.0.0.0:32772->6060/tcp, 0.0.0.0:32776->6060/tcp   tg-lotus-soup-deals-e2e-acfc60bc1727-miners-1
    dff229d7b342        be3c18d7f0d4                "/testplan"              10 seconds ago      Up 9 seconds        0.0.0.0:32778->1234/tcp, 0.0.0.0:32774->2345/tcp, 0.0.0.0:32769->6060/tcp, 0.0.0.0:32770->6060/tcp   tg-lotus-soup-deals-e2e-acfc60bc1727-miners-0
    4cd67690e3b8        be3c18d7f0d4                "/testplan"              11 seconds ago      Up 8 seconds        0.0.0.0:32785->1234/tcp, 0.0.0.0:32780->2345/tcp, 0.0.0.0:32771->6060/tcp, 0.0.0.0:32775->6060/tcp   tg-lotus-soup-deals-e2e-acfc60bc1727-bootstrapper-0
    aeb334adf88d        iptestground/sidecar:edge   "testground sidecar …"   43 hours ago        Up About an hour    0.0.0.0:32768->6060/tcp                                                                              testground-sidecar
    c1157500282b        influxdb:1.8                "/entrypoint.sh infl…"   43 hours ago        Up 25 seconds       0.0.0.0:8086->8086/tcp                                                                               testground-influxdb
    99ca4c07fecc        redis                       "docker-entrypoint.s…"   43 hours ago        Up About an hour    0.0.0.0:6379->6379/tcp                                                                               testground-redis
    bf25c87488a5        bitnami/grafana             "/run.sh"                43 hours ago        Up 26 seconds       0.0.0.0:3000->3000/tcp                                                                               testground-grafana
    cd1d6383eff7        goproxy/goproxy             "/goproxy"               45 hours ago        Up About a minute   8081/tcp                                                                                             testground-goproxy
    ``` 

3. Take note of the port mapping. Imagine in the output above, we want to query
   `54dd5ad916b2`. We'd use `localhost:32788`, as it forwards to the container's
   1234 port (Lotus Full Node RPC).
4. Run your Lotus CLI command setting the `FULLNODE_API_INFO` env variable,
   which is a multiaddr:

   ```sh
   $ FULLNODE_API_INFO=":/ip4/127.0.0.1/tcp/$port/http" lotus chain list
   [...]
   ```

---

Alternatively, you could download gawk and setup a script in you .bashrc or .zshrc similar to:

```
lprt() {
  NAME=$1
  PORT=$2

  docker ps --format "table {{.Names}}" | grep $NAME | xargs -I {} docker port {} $PORT | gawk --field-separator=":" '{print $2}'
}

envs() {
  NAME=$1

  local REMOTE_PORT_1234=$(lprt $NAME 1234)
  local REMOTE_PORT_2345=$(lprt $NAME 2345)

  export FULLNODE_API_INFO=":/ip4/127.0.0.1/tcp/$REMOTE_PORT_1234/http"
  export STORAGE_API_INFO=":/ip4/127.0.0.1/tcp/$REMOTE_PORT_2345/http"

  echo "Setting \$FULLNODE_API_INFO to $FULLNODE_API_INFO"
  echo "Setting \$STORAGE_API_INFO to $STORAGE_API_INFO"
}
```

Then call commands like:
```
envs miners-0
lotus chain list
```

### `cluster:k8s`

Similar to `local:docker`, you pick a pod that you want to connect to and port-forward 1234 and 2345 to that specific pod, such as:

```
export PODNAME="tg-lotus-soup-ae620dfb2e19-miners-0"
kubectl port-forward pods/$PODNAME 1234:1234 2345:2345

export FULLNODE_API_INFO=":/ip4/127.0.0.1/tcp/1234/http"
export STORAGE_API_INFO=":/ip4/127.0.0.1/tcp/2345/http"
lotus-storage-miner storage-deals list
lotus-storage-miner storage-deals get-ask
```

### Useful commands / checks

* **Making sure miners are on the same chain:** compare outputs of `lotus chain list`.
* **Checking deals:** `lotus client list-deals`.
* **Sector queries:** `lotus-storage-miner info` , `lotus-storage-miner proving info`
* **Sector sealing errors:**
    * `STORAGE_API_INFO=":/ip4/127.0.0.1/tcp/53624/http" FULLNODE_API_INFO=":/ip4/127.0.0.1/tcp/53623/http" lotus-storage-miner sector info`
    * `STORAGE_API_INFO=":/ip4/127.0.0.1/tcp/53624/http" FULLNODE_API_INFO=":/ip4/127.0.0.1/tcp/53623/http" lotus-storage-miner sector status <sector_no>`
    * `STORAGE_API_INFO=":/ip4/127.0.0.1/tcp/53624/http" FULLNODE_API_INFO=":/ip4/127.0.0.1/tcp/53623/http" lotus-storage-miner sector status --log <sector_no>`

## Viewing logs of a particular container `local:docker`

This works for both started and stopped containers. Just get the container ID
(in double angle brackets in Testground output, on every log line), and do a:

```shell script
$ docker logs $container_id
```

## Accessing the golang instrumentation

Testground exposes a pprof endpoint under local port 6060, which both
`local:docker` and `cluster:k8s` map.

For `local:docker`, see above to figure out which host port maps to the
container's 6060 port.

## Acquiring a goroutine dump

When things appear to be stuck, get a goroutine dump.

```shell script
$ wget -o goroutine.out http://localhost:${pprof_port}/debug/pprof/goroutine?debug=2
``` 

You can use whyrusleeping/stackparse to extract a summary:

```shell script
$ go get https://github.com/whyrusleeping/stackparse
$ stackparse --summary goroutine.out
```

## Acquiring a CPU profile

When the CPU appears to be spiking/rallying, grab a CPU profile.

```shell script
$ wget -o profile.out http://localhost:${pprof_port}/debug/pprof/profile
``` 

Analyse it using `go tool pprof`. Usually, generating a `png` graph is useful:

```shell script
$ go tool pprof profile.out
File: testground
Type: cpu
Time: Jul 3, 2020 at 12:00am (WEST)
Duration: 30.07s, Total samples = 2.81s ( 9.34%)
Entering interactive mode (type "help" for commands, "o" for options)
(pprof) png
Generating report in profile003.png
```

## Submitting actionable reports / findings

This is useful both internally (within the Oni team, so that peers can help) and
externally (when submitting a finding upstream).

We don't need to play the full bug-hunting game on Lotus, but it's tremendously
useful to provide the necessary data so that any reports are actionable.

These include:

* test outputs (use `testground collect`).
* stack traces that appear in logs (whether panics or not).
* output of relevant Lotus CLI commands.
* if this is some kind of blockage / deadlock, goroutine dumps.
* if this is a CPU hotspot, a CPU profile would be useful.
* if this is a memory issue, a heap dump would be useful.

**When submitting bugs upstream (Lotus), make sure to indicate:**

* Lotus commit.
* FFI commit.
