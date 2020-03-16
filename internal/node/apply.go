package node

import (
    "log"

    "github.com/slin63/raft-consensus/internal/spec"
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
    log.Printf("TODO: Apply(). Currently mocking. [idx=%d]", idx)
    raft.CommitIndex = idx
    return true
}
