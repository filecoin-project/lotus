#!/usr/bin/env bash

GRAFANA_HOST="localhost:13000"

curl -XPOST http://admin:admin@$GRAFANA_HOST/api/datasources -H 'Content-Type: text/json' --data-binary @- > /dev/null << EOF
{
  "name":"ntwk-localstats",
  "type":"influxdb",
  "database":"lotus",
  "url": "http://influxdb:8086",
  "basicAuth":false,
  "access": "proxy"
}
EOF

curl -XPOST http://admin:admin@$GRAFANA_HOST/api/dashboards/import -H 'Content-Type: text/json' --data-binary @- << EOF | jq -r "\"http://$GRAFANA_HOST\" + .importedUrl"
{
  "dashboard": $(cat ./chain.dashboard.json),
  "overwrite": true,
  "inputs": [
    {
      "name": "DS_INFLUXDB",
      "pluginId": "influxdb",
      "type": "datasource",
      "value": "InfluxDB"
    }
  ]
}
EOF
