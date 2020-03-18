// Code for communicating with the client
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

type entryC struct {
    // An entry
    D string

    // Channel that digestEntries will use to notify upstream
    // PutEntry of successful replication
    C chan *responses.Result
}

// Receive entries from a client to be added to our log
//  - It is up to our downstream client to to determine whether
//    or not it's a valid entry.
//  - The leader appends the command to its log as a new entry,
//    then issues AppendEntries RPCs in parallel to each of the
//    other servers to replicate the entry.
//  - digestEntries processes entries chan and on successful replication,
//    sends a success signal back upstream to PutEntry to indicate the entry
//      a. was safely replicated
//      b. will now be applied to the state machine
func (f *Ocean) PutEntry(entry string, result *responses.Result) error {
    log.Printf("PutEntry(): %s", tr(entry, 15))
    resp := make(chan *responses.Result)

    // Add new entry to log for processing
    entries <- entryC{entry, resp}

    select {
    case r := <-resp:
        r.Entry = entry
        if r.Success {
            // The entry was successfully processed.
            // 1. Apply to our own state.
            // The program will explode if the state application fails.
            commits <- r.Index
        }
        *result = *r
    case <-time.After(time.Second * time.Duration(config.C.RPCTimeout)):
        config.LogIf(fmt.Sprintf("[PUTENTRY]: PutEntry timed out waiting for quorum"), config.C.LogPutEntry)
        *result = responses.Result{Term: raft.CurrentTerm, Success: false}
    }

    return nil
}

func digestCommits() {
    // Digest commits in order. Applies that fail crash the server
    for idx := range commits {
        config.LogIf(fmt.Sprintf("[APPLY]: Applying index %d", idx), config.C.LogDigestCommits)
        if ok := applyCommits(idx); !ok {
            log.Fatalf("[APPLY-X] Failed to apply commit [idx=%d]. Terminating.", idx)
        } else {
            config.LogIf(fmt.Sprintf("[APPLY]: Successfully applied index %d", idx), config.C.LogDigestCommits)
        }
    }
}

func digestEntries() {
    // Digest client entries in order
    for entry := range entries {
        var once sync.Once
        // Add new entry to own log
        spec.RaftRWMutex.Lock()
        idx := raft.AppendEntry(entry.D)
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
                config.LogIf(fmt.Sprintf("[DIGESTENTRIES] QUOROM received (%d/%d) [entry=%s]", rcount, quorum, tr(entry.D, 10)), config.C.LogDigestEntries)
                once.Do(func() { entry.C <- r })
            }
        }
    }
}

// As titled. Assume the following:
// 1) For any server capable of responding, we will EVENTUALLY receive
//    a successful response.
func appendEntriesUntilSuccess(raft *spec.Raft, PID int) *responses.Result {
    var result *responses.Result
    // If last log index >= nextIndex for a follower,
    // send log entries starting at nextIndex
    if len(raft.Log)-1 >= raft.NextIndex[PID] {
        for {
            // Regenerate arguments on each call, because
            // raft state may have changed between calls
            spec.RaftRWMutex.RLock()
            args := raft.GetAppendEntriesArgs(&self)
            args.PrevLogIndex = raft.NextIndex[PID] - 1
            args.PrevLogTerm = spec.GetTerm(&raft.Log[args.PrevLogIndex])
            args.Entries = raft.Log[raft.NextIndex[PID]:]
            config.LogIf(
                fmt.Sprintf("appendEntriesUntilSuccess() with args: %v, %v, %v, %v, %v",
                    args.Term,
                    args.LeaderId,
                    args.PrevLogIndex,
                    args.PrevLogTerm,
                    args.LeaderCommit,
                ),
                config.C.LogAppendEntries)
            spec.RaftRWMutex.RUnlock()
            result = CallAppendEntries(PID, args)

            // Success! Increment next/matchIndex as a function of our inputs
            // Otherwise, decrement nextIndex and try again.
            spec.RaftRWMutex.Lock()
            defer spec.RaftRWMutex.Unlock()

            if result.Success {
                raft.MatchIndex[PID] = args.PrevLogIndex + len(args.Entries)
                raft.NextIndex[PID] = raft.MatchIndex[PID] + 1
                break
            } else {
                // Decrement NextIndex if the failure was due to log consistency.
                // If not, update our term and step down
                if result.Term > raft.CurrentTerm {
                    raft.CurrentTerm = result.Term
                    raft.Role = spec.FOLLOWER
                } else {
                    raft.NextIndex[PID] -= 1
                }
            }
        }
    } else {
        log.Printf("log length was weird")
    }
    log.Printf("[PUTENTRY->]: [PID=%d]", PID)
    return result
}
