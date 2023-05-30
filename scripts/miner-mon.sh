#!/usr/bin/env bash

SESSION=$(cat /proc/sys/kernel/random/uuid)

tmux -2 new-session -d -s $SESSION

tmux new-window -t $SESSION:1 -n 'Miner'

tmux split-window -h

tmux select-pane -t 0
tmux send-keys "watch -n1 './lotus-miner info'" C-m

tmux split-window -v

tmux select-pane -t 1
tmux send-keys "watch -n1 './lotus-miner workers list'" C-m

tmux select-pane -t 2
tmux send-keys "watch -n1 './lotus-miner storage list'" C-m


tmux -2 attach-session -t $SESSION
