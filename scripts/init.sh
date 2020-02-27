#!/bin/sh
trap 'kill $(jobs -p)' TERM

# start service in background here
./main &
go run chord-failure-detector/src/main.go &

wait
