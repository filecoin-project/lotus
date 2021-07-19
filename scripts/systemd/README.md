# filecoin-systemd

### DO A COPY
```
sudo cp *.service /etc/systemd/system
```

### Start Lotus Daemon
```
sudo systemctl enable --now lotus-daemon.service
```

### Start Lotus Miner
```
sudo systemctl enable --now lotus-miner.service
```

### Start one P1
```
sudo systemctl enable --now lotus-worker-p1@0.service
```

### Start two P2
```
for ID in $(seq 0 1)
do
  sudo systemctl enable --now lotus-worker-p2@${ID}.service
done
```

### Start six C2
```
for ID in $(seq 0 5)
do
  sudo systemctl enable --now lotus-worker-c2@${ID}.service
done
```

---

# OR use systemd target unit

### DO A COPY
```
sudo cp *.service /etc/systemd/system
sudo cp *.target /etc/systemd/system
```

### Start Lotus Daemon
```
sudo systemctl enable --now lotus-daemon.service
```

### Start Lotus Miner
```
sudo systemctl enable --now lotus-miner.service
```

### Start one P1
```
sudo systemctl enable --now lotus-worker-p1s.target
```

### Start two P2
```
sudo systemctl enable --now lotus-worker-p2s.target
```

### Start six C2
```
sudo systemctl enable --now lotus-worker-c2s.target
```
