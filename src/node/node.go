// Stuff the server needs to do to stay alive and do its job.
package node

import (
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"../config"
	"../spec"
)

// Raft state
var raft *spec.Raft
var leader bool

// Membership layer state
var self spec.Self

var block = make(chan int, 1)

// A channel full of unix timestamps corresponding to last heartbeats
var heartbeats = make(chan int64, 1)

func Live(isLeader bool) {
	// If you're the leader, sleep for a little bit and let everyone else get started.
	// TODO (03/03 @ 11:07): one election is implemented, have it so that the nodes elect a leader
	// instead of having a "default" leader
	if isLeader {
		time.Sleep(time.Second * 1)
	}

	// Initialize logging to file
	f, err := os.OpenFile(config.C.Logfile, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		log.Fatalf("error opening file: %v", err)
	}
	defer f.Close()

	// Get initial membership info
	spec.GetSelf(&self)
	log.SetPrefix(config.C.Prefix + fmt.Sprintf(" [PID=%d]", self.PID) + " - ")

	// Create our raft instance
	leader = isLeader
	raft = &spec.Raft{
		Log:          []string{"0,NULL"},
		ElectTimeout: spec.ElectTimeout(),
		Wg:           &sync.WaitGroup{},
	}
	raft.Init(&self)
	if leader {
		raft.Role = spec.LEADER
	}

	spec.ReportOnline()

	go serveOceanRPC()
	go subscribeMembership()
	go heartbeat()

	<-block
}

// Leaders beat their hearts
// Followers listen to heartbeats and initiate elections after enough time is passed
func heartbeat() {
	var wg sync.WaitGroup
	var last int64

	for {
		if leader {
			// Send empty append entries to every member as goroutines
			spec.SelfRWMutex.RLock()
			for PID := range self.MemberMap {
				if PID != self.PID {
					wg.Add(1)
					go func(PID int, wg *sync.WaitGroup) {
						// TODO (03/03 @ 11:07): may have to replace this with appendEntriesUntilSuccess
						CallAppendEntries(PID, raft.GetAppendEntriesArgs(&self))
						config.LogIf(
							fmt.Sprintf("[HEARTBEAT->]: [PID=%d]", PID),
							config.C.LogHeartbeats,
						)
						wg.Done()
					}(PID, &wg)
				}
			}
			spec.SelfRWMutex.RUnlock()

			// Wait for those routines to complete
			wg.Wait()
			time.Sleep(time.Duration(config.C.HeartbeatInterval) * time.Millisecond)
		} else {
			if len(heartbeats) > 0 {
				last = <-heartbeats
			}
			if last != 0 && timeMs()-last > raft.ElectTimeout {
				log.Fatalf("[ELECTTIMEOUT]")
			}
		}
	}
}

// Periodically poll for membership information
func subscribeMembership() {
	for {
		spec.GetSelf(&self)
		time.Sleep(time.Second * spec.MemberInterval)
	}
}

// // Detect ctrl-c signal interrupts and dispatch [LEAVE]s to monitors accordingly
// func listenForLeave() {
// 	c := make(chan os.Signal, 2)
// 	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
// 	go func() {
// 		<-c
// 		os.Exit(0)
// 	}()
// }
