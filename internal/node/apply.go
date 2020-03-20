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
func applyCommits(idx int) *responses.Result {
    var r *responses.Result
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
        return &responses.Result{Success: false}
    }
    defer client.Close()

    // Entries to apply to state machine
    // Only return entry whose index matches `idx`
    start := raft.CommitIndex + 1 // inclusive
    end := idx + 1                // exclusive
    current := start
    config.LogIf(fmt.Sprintf("[APPLY]: Applying %d entries", start-end), config.C.LogDigestCommits)
    for _, entry := range raft.Log[start:end] {

        var result responses.Result
        if err := (*client).Call("Filesystem.Execute", entry, &result); err != nil {
            log.Fatal(err)
        }
        if current == idx {
            r = &result
            config.LogIf(
                fmt.Sprintf("[APPLY]: Found match for requested [IDX=%d]", idx),
                config.C.LogDigestCommits)
        }
        config.LogIf(
            fmt.Sprintf("[APPLY]: Result for %s: S:%v D:%v E:%v I:%v",
                tr(entry, 10),
                result.Success,
                result.Data,
                tr(result.Entry, 20),
                result.Index),
            config.C.LogDigestCommits,
        )
    }

    raft.CommitIndex = idx
    return r
}
