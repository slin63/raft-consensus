// Code for communicating with the client
package node

import (
    "log"
    "time"

    "github.com/slin63/raft-consensus/internal/config"
    "github.com/slin63/raft-consensus/internal/spec"
)

type entryC struct {
    Entry string
    C     chan *spec.Result
}

// Receive entries from a client to be added to our log
// It is up to our downstream client to to determine whether
// or not it's a valid entry
// The leader appends the command to its log as a new entry,
// then issues AppendEntries RPCs in parallel to each of the
// other servers to replicate the entry.
func (f *Ocean) PutEntry(entry string, result *spec.Result) error {
    log.Printf("PutEntry(): %s", entry)
    resp := make(chan *spec.Result)

    // Add new entry to log for processing
    entries <- entryC{entry, resp}
    select {
    case r := <-resp:
        log.Println(r)
        *result = r
    case <-time.After(time.Second * time.Duration(config.C.RPCTimeout)):
        log.Printf("whoops")
        *result = spec.Result{Term: raft.CurrentTerm, Success: false}
    }

    return nil
}

func digestEntries() {
    for entry := range resp {
        // Dispatch AppendEntries to follower nodes
        spec.SelfRWMutex.RLock()
        for PID := range self.MemberMap {
            if PID != self.PID {
                go appendEntriesUntilSuccess(raft, PID)
            }
        }
        spec.SelfRWMutex.RUnlock()

    }

}
