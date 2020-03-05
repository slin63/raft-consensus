#!/bin/sh
trap 'kill $(jobs -p)' TERM

# start service in background here
./raft &
./member &

wait
