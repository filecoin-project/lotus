# lotus-stats

`lotus-stats` is a small tool to push chain information into influxdb

## Setup

Influx configuration can be configured through env variables.

```
INFLUX_ADDR="http://localhost:8086"
INFLUX_USER=""
INFLUX_PASS=""
```

## Usage

lotus-stats will be default look in `~/.lotus` to connect to a running daemon and resume collecting stats from last record block height.

For other usage see `./lotus-stats --help`

```
go build -o lotus-stats *.go 
. env.stats && ./lotus-stats
```


## Development

Start grafana and influxdb containers and import the dashboard to grafana.
The url of the imported dashboard will be returned.

If the script doesn't work, you can manually setup the datasource and import the dashboard.

```
docker-compose up -d
./setup.bash
```

The default username and password for grafana are both `admin`.
