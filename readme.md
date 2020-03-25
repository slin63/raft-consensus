# Raft

### Running clusters
1. Start up server cluster
  - `docker-compose build && docker-compose up --remove-orphans --scale worker=3`
2. Send whatever commands you want with
  - `CONFIG=$(pwd)/config.json LEADER=0 CLIENT=1 go run ./cmd/raft`
3. Be confused
4. Rainbows!

### Running tests
1. `CONFIG=$(pwd)/config.json go test -v ./internal/...`
