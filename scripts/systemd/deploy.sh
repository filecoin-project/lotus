# vim:set ts=2 sts=2 sw=2 et:
#!/bin/bash

# Replace WORKER_IP
IFACE=eth0
IPADDR=$(ip a show ${IFACE}|grep 'inet '|awk '{print $2}'|sed 's/\/.*//')

sudo sed -i "s/WORKER_IP=.*/WORKER_IP=${IPADDR}/g" /etc/systemd/system/lotus-worker-p1@.service
sudo sed -i "s/WORKER_IP=.*/WORKER_IP=${IPADDR}/g" /etc/systemd/system/lotus-worker-p2@.service
sudo sed -i "s/WORKER_IP=.*/WORKER_IP=${IPADDR}/g" /etc/systemd/system/lotus-worker-c2@.service
