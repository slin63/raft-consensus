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
	// Create our raft instance
	leader = isLeader
	raft = &spec.Raft{
		Log:          []string{"0"},
		ElectTimeout: spec.ElectTimeout(),
		NextIndex:    2,
		Wg:           &sync.WaitGroup{},
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
					go CallAppendEntries(PID, &spec.AppendEntriesArgs{}, &wg)
					config.LogIf(
						fmt.Sprintf("[HEARTBEAT->]: [PID=%d]", PID),
						config.C.LogHeartbeats,
					)
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
