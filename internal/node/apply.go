package node

import (
    "fmt"
    "log"
    "sync"
    "time"

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

func digestCommits() {
    // Digest commits in order.
    //   - Applies that fail crash the server
    //   - Entries that are not successful are sent downstream
    for commit := range commits {
        config.LogIf(fmt.Sprintf("[APPLY]: Applying index %d", commit.Idx), config.C.LogDigestCommits)
        r := applyCommits(commit.Idx)
        config.LogIf(fmt.Sprintf("[APPLY]: Successfully applied index %d", commit.Idx), config.C.LogDigestCommits)

        select {
        case commit.C <- r:
        case <-time.After(time.Second * time.Duration(config.C.RPCTimeout)):
            config.LogIf(fmt.Sprintf("[DIGESTCOMMITS]: Response channel blocked"), config.C.LogDigestCommits)
        }
    }
}

func digestEntries() {
    // Digest client entries in order
    for entry := range entries {
        var once sync.Once
        // Add new entry to own log
        spec.RaftRWMutex.Lock()
        config.LogIf(fmt.Sprintf("[DIGESTENTRIES]: Processing [%s]", entry.D), config.C.LogDigestEntriesVerbose)
        idx := raft.AppendEntry(entry.D)
        config.LogIf(fmt.Sprintf("[DIGESTENTRIES]: New log %s", raft.Log), config.C.LogDigestEntriesVerbose)
        spec.RaftRWMutex.Unlock()

        rch := make(chan *responses.Result)
        rcount := 0

        // Dispatch AppendEntries to follower nodes
        spec.SelfRWMutex.RLock()
        quorum := spec.GetQuorum(&self)
        remaining := len(self.MemberMap) - 1
        for PID := range self.MemberMap {
            if PID != self.PID {
                go func(PID int, remaining *int) {
                    r := appendEntriesUntilSuccess(raft, PID)
                    r.Index = idx
                    rch <- r
                    if *remaining -= 1; *remaining == 0 {
                        close(rch)
                    }
                }(PID, &remaining)
            }
        }
        spec.SelfRWMutex.RUnlock()

        // Parse responses from servers and notify RPC about safely committed entries.
        for r := range rch {
            if !r.Success {
                panic("appendEntriesUntilSuccess should never fail")
            }
            rcount += 1
            if rcount >= quorum {
                config.LogIf(fmt.Sprintf("[DIGESTENTRIES] QUOROM received (%d/%d) [entry=%s]", rcount, quorum, tr(entry.D, 20)), config.C.LogDigestEntries)
                once.Do(func() { entry.C <- r })
            }
        }
    }
}
