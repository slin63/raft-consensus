// Stuff the server needs to do to stay alive and do its job.
package node

import (
    "fmt"
    "io"
    "log"
    "os"
    "runtime"
    "sync"
    "time"

    "github.com/slin63/raft-consensus/internal/config"
    "github.com/slin63/raft-consensus/internal/spec"
    "github.com/slin63/raft-consensus/pkg/responses"
)

// Raft state
var raft *spec.Raft

// Membership layer state
var self spec.Self

var entries = make(chan entryC)
var commits = make(chan commitC)
var heartbeats = make(chan int)
var membershipUpdate = make(chan struct{})
var endElection = make(chan int)
var block = make(chan int, 1)

func Live() {
    // Initialize logging to file
    f, err := os.OpenFile(config.C.Logfile, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
    if err != nil {
        log.Fatalf("error opening file: %v", err)
    }
    defer f.Close()

    mw := io.MultiWriter(os.Stdout, f)
    log.SetOutput(mw)

    // Get initial membership info
    rejoin := spec.GetSelf(&self)
    log.SetPrefix(config.C.Prefix + fmt.Sprintf(" [PID=%d]", self.PID) + " - ")

    // Create our raft instance
    raft = &spec.Raft{
        Log:          []string{"0,NULL"},
        ElectTimeout: spec.ElectTimeout(),
        Wg:           &sync.WaitGroup{},
    }
    raft.Init(&self)

    spec.ReportOnline(raft.ElectTimeout)
    go live(rejoin)
    <-block
}

func live(rejoin bool) {
    go serveOceanRPC()
    go subscribeMembership(membershipUpdate)
    go heartbeat()
    go dispatchHeartbeats()
    go digestEntries()
    go digestCommits()

    if config.C.LogGoroutines {
        go logGoroutines()
    }

    // Wait for other nodes to come online.
    // Wait extra if you're rejoining the cluster so you can get all
    //   the logs and reapplying your state.
    // Start the election timer
    if rejoin {
        config.LogIf(fmt.Sprintf("[RESTORE]"), config.C.LogRestore)
        time.Sleep(time.Second * time.Duration(config.C.RestoreWait))
        commCh := make(chan *responses.Result)
        raft.CommitIndex = 0
        commits <- commitC{len(raft.Log) - 1, commCh}
        log.Printf("[CommitIndex=%d]", raft.CommitIndex)

        select {
        case <-commCh:
            log.Printf("[RESTORE] COMPLETE")
        case <-time.After(time.Second * time.Duration(config.C.RestoreTimeout)):
            config.LogIf(fmt.Sprintf("[RESTORE]: State restore timed out."), config.C.LogRestore)
        }
    }
    time.Sleep(time.Second * 1)
    raft.ResetElectTimer()
}

// Leaders beat their hearts
// Followers listen to heartbeats and initiate elections after enough time is passed
func heartbeat() {
    for {
        if raft.Role == spec.LEADER {
            // Send empty append entries to every member as goroutines
            for PID := range self.MemberMap {
                if PID != self.PID {
                    heartbeats <- PID
                }
            }
            time.Sleep(time.Duration(config.C.HeartbeatInterval) * time.Millisecond * time.Duration(config.C.Timescale))
        } else {
            // Watch for election timer timeouts
            select {
            case <-raft.ElectTimer.C:
                if raft.Role == spec.CANDIDATE {
                    config.LogIf(fmt.Sprintf("[ELECTTIMEOUT] CANDIDATE timed out while waiting for votes"), config.C.LogElections)
                } else {
                    log.Println("[ELECTTIMEOUT]")
                }
                go InitiateElection()

            default:
                continue
            }
        }
    }
}

// Check safety of heartbeat target and dispatch heartbeat goroutine
func dispatchHeartbeats() {
    for PID := range heartbeats {
        if _, ok := self.MemberMap[PID]; !ok {
            config.LogIf(
                fmt.Sprintf("[HEARTBEATERR] Tried heartbeating to dead node [PID=%d].", PID),
                config.C.LogHeartbeats,
            )
            return
        } else {
            go func(PID int) {
                config.LogIf(
                    fmt.Sprintf("[TERM=%d] [HEARTBEAT->]: to [PID=%d]", raft.CurrentTerm, PID),
                    config.C.LogHeartbeatsLead,
                )
                r := CallAppendEntries(PID, raft.GetAppendEntriesArgs(&self))
                if !r.Success && r.Error == responses.MISSINGLOGENTRY {
                    config.LogIf(fmt.Sprintf("[RETRYAPPENDENTRY] [PID=%d]", PID), config.C.LogConflictingEntries)
                    r = appendEntriesUntilSuccess(raft, PID)
                }
                config.LogIf(
                    fmt.Sprintf("[TERM=%d] [HEARTBEAT->]: DONE FROM [PID=%d]", raft.CurrentTerm, PID),
                    (config.C.LogHeartbeatsLead && r.Error != responses.CONNERROR),
                )
            }(PID)
        }
    }

}

// Periodically poll for membership information
func subscribeMembership(membershipUpdate chan<- struct{}) {
    for {
        spec.GetSelf(&self)
        time.Sleep(time.Second * time.Duration(config.C.MemberInterval))
    }
}

func logGoroutines() {
    for {
        log.Printf("[GOROUTINES]: %d", runtime.NumGoroutine())
        time.Sleep(time.Second * 5)
    }
}
