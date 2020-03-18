package node

import (
    "fmt"
    "log"

    "github.com/slin63/raft-consensus/internal/config"
    "github.com/slin63/raft-consensus/internal/spec"
    "github.com/slin63/raft-consensus/pkg/responses"
)

// Apply any uncommitted changes to our state
//   - Tries to apply changes up to idx.
//   - For change in range(r.CommitIndex, idx):
//       - Tries to apply change. Terminates program on failure.
//       - Updates r.CommitIndex = idx
// TODO (03/15 @ 13:38): Hook up after DFS is working sort of.
func applyCommits(idx int) bool {
    spec.RaftRWMutex.Lock()
    defer spec.RaftRWMutex.Unlock()

    // Try to establish connection to DFS.
    config.LogIf(
        fmt.Sprintf("applyCommits() trying to connect to PID %d", self.PID),
        config.C.LogConnections)
    client, err := connect(self.PID, config.C.FilesystemRPCPort)
    if err != nil {
        config.LogIf(
            fmt.Sprintf("[CONNERROR] applyCommits failed to connect to [PID=%d]. Aborting", self.PID),
            config.C.LogConnections)
        return false
    }
    defer client.Close()

    // Entries to apply to state machine
    toApply := raft.Log[raft.CommitIndex+1 : idx+1]
    config.LogIf(fmt.Sprintf("[APPLY]: Applying %d entries", len(toApply)), config.C.LogDigestCommits)
    for _, entry := range toApply {
        var result responses.Result
        if err := (*client).Call("Filesystem.Execute", entry, &result); err != nil {
            log.Fatal(err)
        }
        config.LogIf(fmt.Sprintf("[APPLY]: Result for %s: %v", tr(entry, 10), result), config.C.LogDigestCommits)
    }

    raft.CommitIndex = idx

    return true
}
