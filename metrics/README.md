# Setting Up Prometheus and Grafana

Lotus supports exporting a wide range of metrics, enabling users to gain insights into its behavior and effectively analyze performance issues. These metrics can be conveniently utilized with aggregation and visualization tools for in-depth analysis. In this document, we show how you can set up Prometheus and Grafana for monitoring and visualizing these metrics:

- **Prometheus**: Prometheus is an open-source monitoring and alerting toolkit designed for collecting and storing time-series data from various systems and applications. It provides a robust querying language (PromQL) and a web-based interface for analyzing and visualizing metrics.

- **Grafana**: Grafana is an open-source platform for creating, sharing, and visualizing interactive dashboards and graphs. It integrates with various data sources, including Prometheus, to help users create meaningful visual representations of their data and set up alerting based on specific conditions.

## Prerequisites

- You have a Linux or Mac based system.
- You have root access to install software
- You have lotus node already running

**Note:** These instructions have been tested on Ubuntu 23.04 and on Mac M1.

## Install and start Prometheus

### On Ubuntu:

```
# install prometheus
sudo apt-get install prometheus

# copy the prometheus.yml config to the correct directory
sudo cp metrics/prometheus.yml /etc/prometheus/prometheus.yml

# start prometheus
sudo systemctl start prometheus

# enable prometheus on boot (optional)
sudo systemctl enable prometheus
```

### On Mac:

```
# install prometheus
brew install prometheus

# start prometheus
prometheus --config.file=lotus/metrics/prometheus.yml
```

## Install and start Grafana

### On Ubuntu:

```
# download the Grafana GPG key in our keyring
wget -q -O - https://packages.grafana.com/gpg.key | gpg --dearmor | sudo tee /usr/share/keyrings/grafana.gpg > /dev/null

# add the Grafana repository to our APT sources
echo "deb [signed-by=/usr/share/keyrings/grafana.gpg] https://packages.grafana.com/oss/deb stable main" | sudo tee -a /etc/apt/sources.list.d/grafana.list

# update our APT cache
sudo apt-get update

# now we can install grafana
sudo apt-get install grafana

# start grafana
sudo systemctl start grafana-server

# start grafana on boot (optional)
sudo systemctl enable grafana-server
```

### On Mac:

```
brew install grafana
brew services start grafana
```

You should now have Prometheus and Grafana running on your machine where Prometheus is already collecting metrics from your Lotus node (if its running) and saving it to a database.

You can confirm everything is setup correctly by visiting:
- Prometheus (http://localhost:9090): You can open the metric explorer and view any of the aggregated metrics scraped from Lotus
- Grafana (http://localhost:3000): Default username/password is admin/admin, remember to change it after login.

## Add Prometheus as datasource in Grafana

1. Log in to Grafana using the web interface.
2. Navigate to "Home" > "Connections" > "Data Sources."
3. Click "Add data source."
4. Choose "Prometheus."
5. In the "HTTP" section, set the URL to http://localhost:9090.
6. Click "Save & Test" to verify the connection.

## Import one of the existing dashboards in lotus/metrics/grafana

1. Log in to Grafana using the web interface.
2. Navigate to "Home" > "Dashboards" > Click the drop down menu in the "New" button and select "Import"
3. Paste any of the existing dashboards in lotus/metrics/grafana into the "Import via panel json" panel.
4. Click "Load"
5. Select the Prometheus datasource you created earlier
6. Click "Import"

### Available Dashboards

The repository includes several pre-configured dashboards that can be imported:

- **F3Dashboard.json**: A comprehensive dashboard for monitoring F3 metrics, including GPBFT instance completion rates, error counts by type, phase transitions, and encoding performance. This is ideal for monitoring the health and performance of your F3 system.
- **MessageExecution.json**: Provides insights into message execution metrics, with a focus on ApplyBlocks timing data.

# Collect system metrics using node_exporter

Although Lotus includes many useful metrics it does not include system metrics, such as information about cpu, memory, disk, network, etc. If you are investigating an issue and have Lotus metrics available, its often very useful to correlate certain events or behaviour with general system metrics.

## Install node_exporter
If you have followed this guide so far and have Prometheus and Grafana already running, you can run the following commands to also aggregate the system metrics:

Ubuntu:

```

# download the newest release by https://github.com/prometheus/node_exporter/releases (it was 1.6.1 as of writing this doc)
wget https://github.com/prometheus/node_exporter/releases/download/v1.6.1/node_exporter-1.6.1.linux-amd64.tar.gz

# extract the release (it contains a single binary plus some docs)
tar -xf node_exporter-1.6.1.linux-amd64.tar.gz

# move it to /usr/local/bin
sudo mv node_exporter-1.6.1.linux-amd64/node_exporter /usr/local/bin

# run node_exporter
node_exporter
```

Mac:

```
# install node_exporter
brew install node_exporter

# run node_exporter
node_exporter
```

## Import system dashboard

Since our `prometheus.yml` config already has configuration for node_exporter, we can go straight away and import a Grafana dashboard for viewing:

1. Download the most recent dashboard from https://grafana.com/grafana/dashboards/1860-node-exporter-full/
2. Log in to Grafana (http://localhost:3000) using the web interface.
3. Navigate to "Home" > "Dashboards" > Click the drop down menu in the "New" button and select "Import"
4. Paste any of the existing dashboards in lotus/metrics/grafana into the "Import via panel json" panel.
5. Click "Load"
6. Select the Prometheus datasource you created earlier
7. Click "Import"
