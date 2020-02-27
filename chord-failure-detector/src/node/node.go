package node

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/rpc"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"

	"../config"
	"../hashing"
	"../spec"
)

var selfIP string
var selfPID int

// [PID:*memberNode]

var memberMap = make(map[int]*spec.MemberNode)

// [PID:Unix timestamp at time of death]
// Assume all PIDs here point to dead nodes, waiting to be deleted

var suspicionMap = make(map[int]int64)

// [finger:PID]
var fingerTable = make(map[int]int)

var joinReplyLen = 15
var joinReplyChan = make(chan int, joinReplyLen)

const joinReplyInterval = 3
const joinAttemptInterval = 20
const heartbeatInterval = 5

const m int = 8
const RPCPort = 6002
const introducerPort = 6001
const heartbeatPort = 6000
const delimiter = "//"
const logHeartbeats = false

var heartbeatAddr net.UDPAddr
var RPCAddr net.TCPAddr

// RPCs
type Membership int
type SuspicionMapT map[int]int64
type FingerTableT map[int]int
type MemberMapT map[int]*spec.MemberNode
type Self struct {
	M            int
	PID          int
	MemberMap    MemberMapT
	FingerTable  FingerTableT
	SuspicionMap SuspicionMapT
}

func Live(introducer bool, logf string) {
	selfIP = getSelfIP()
	selfPID = hashing.MHash(selfIP, m)
	spec.ReportOnline(selfIP, selfPID, introducer)

	// So the program doesn't die
	var wg sync.WaitGroup
	wg.Add(1)

	// Initialize logging to file
	f, err := os.OpenFile(logf, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		log.Fatalf("error opening file: %v", err)
	}
	defer f.Close()

	mw := io.MultiWriter(os.Stdout, f)
	log.SetOutput(mw)

	// Initialize addresses
	heartbeatAddr = net.UDPAddr{
		IP:   net.ParseIP(selfIP),
		Port: heartbeatPort,
	}
	RPCAddr = net.TCPAddr{
		IP:   net.ParseIP(selfIP),
		Port: RPCPort,
	}

	// Listen for messages
	go listen()

	// Join the network if you're not the introducer
	if !introducer {
		joinNetwork()
	} else {
		go listenForJoins()
		go dispatchJoinReplies()
	}

	// Beat that drum
	go heartbeat(introducer)

	// Listen for leaves
	go listenForLeave()

	// Serve RPCs
	go serveRPCs()

	wg.Wait()
}

// Listen function specifically for JOINs.
func listenForJoins() {
	p := make([]byte, 128)
	ser, err := net.ListenUDP("udp", &net.UDPAddr{
		IP:   net.ParseIP(selfIP),
		Port: introducerPort,
	})
	if err != nil {
		log.Fatal(err)
	}

	// Begin the UDP listen loop
	for {
		_, remoteaddr, err := ser.ReadFromUDP(p)
		if err != nil {
			log.Fatal(err)
		}

		var bb [][]byte = bytes.Split(p, []byte(delimiter))
		replyCode, err := strconv.Atoi(string(bb[0][0]))
		if err != nil {
			log.Fatal(err)
		}

		switch replyCode {
		case spec.JOIN:
			// Update our own member map & fts
			newPID := hashing.MHash(remoteaddr.IP.String(), m)

			// Check for potential collisions / outdated memberMap
			if node, exists := memberMap[newPID]; exists {
				log.Printf(
					"[COLLISION] PID %v for %v collides with existing node at %v. Try raising m to allocate more ring positions. (m=%v)",
					newPID,
					remoteaddr,
					node.IP,
					m,
				)
			}
			newNode := &spec.MemberNode{
				IP:        remoteaddr.IP.String(),
				Timestamp: time.Now().Unix(),
				Alive:     true,
			}

			spec.SetMemberMap(newPID, newNode, &memberMap)
			spec.RefreshMemberMap(selfIP, selfPID, &memberMap)
			spec.ComputeFingerTable(&fingerTable, &memberMap, selfPID, m)
			// Add message to queue:
			// Send the joiner a membership map so that it can discover more peers.
			joinReplyChan <- newPID
			log.Printf(
				"[JOIN] (PID=%d) (IP=%s) (T=%d) joined network. Added to memberMap & FT.",
				newPID,
				(*newNode).IP,
				(*newNode).Timestamp,
			)
		}
	}
}

// Send out joinReplyLen / 2 [JOINREPLY] messages every joinReplyInterval seconds
func dispatchJoinReplies() {
	for {
		for i := 0; i < (joinReplyLen / 2); i++ {
			PID := <-joinReplyChan
			log.Printf("[JOINREPLY]: Dispatching message for [PID=%d]", PID)
			sendMessage(
				PID,
				fmt.Sprintf("%d%s%s", spec.JOINREPLY, delimiter, spec.EncodeMemberMap(&memberMap)),
			)
		}
		time.Sleep(joinReplyInterval * time.Second)
	}
}

// Listen function to handle: HEARTBEAT, JOINREPLY
func listen() {
	log.Printf("[PID=%d] is listening!", selfPID)
	var p [1024]byte

	ser, err := net.ListenUDP("udp", &heartbeatAddr)
	if err != nil {
		log.Fatal("listen(): ", err)
	}

	for {
		n, _, err := ser.ReadFromUDP(p[0:])
		if err != nil {
			log.Fatal(err)
		}

		// Identify appropriate protocol via message code and react
		var bb [][]byte = bytes.Split(p[0:n], []byte(delimiter))
		replyCode, err := strconv.Atoi(string(bb[0][0]))
		if err != nil {
			log.Fatal(err)
		}

		switch replyCode {
		// We successfully joined the network
		// Decode the membership gob and merge with our own membership list.
		case spec.JOINREPLY:
			theirMemberMap := spec.DecodeMemberMap(bb[1])
			spec.MergeMemberMaps(&memberMap, &theirMemberMap)
			spec.ComputeFingerTable(&fingerTable, &memberMap, selfPID, m)
			log.Printf(
				"[JOINREPLY] Successfully joined network. Discovered %d peer(s).",
				len(memberMap)-1,
			)
		case spec.HEARTBEAT:
			theirMemberMap := spec.DecodeMemberMap(bb[1])
			spec.MergeMemberMaps(&memberMap, &theirMemberMap)
			spec.ComputeFingerTable(&fingerTable, &memberMap, selfPID, m)
		case spec.LEAVE:
			leavingPID, err := strconv.Atoi(string(bb[1]))
			leavingTimestamp, err := strconv.Atoi(string(bb[2]))
			if err != nil {
				log.Fatalf("[LEAVE]: %v", err)
			}

			leaving, ok := memberMap[leavingPID]
			if !ok {
				log.Fatalf("[LEAVE] PID=%s not in memberMap", leavingPID)
			}

			// Add to suspicionMap so that none-linked nodes will eventually hear about this.
			leavingCopy := *leaving
			leavingCopy.Alive = false
			spec.SetSuspicionMap(leavingPID, int64(leavingTimestamp), &suspicionMap)
			spec.SetMemberMap(leavingPID, &leavingCopy, &memberMap)
			log.Printf("[LEAVE] from PID=%d (timestamp=%d)", leavingPID, leavingTimestamp)
		default:
			log.Printf("[NOACTION] Received replyCode: [%d]", replyCode)
		}
	}
}

// Detect ctrl-c signal interrupts and dispatch [LEAVE]s to monitors accordingly
func listenForLeave() {
	c := make(chan os.Signal, 2)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		message := fmt.Sprintf(
			"%d%s%d%s%d",
			spec.LEAVE, delimiter,
			selfPID, delimiter,
			time.Now().Unix(),
		)
		spec.Disseminate(
			message,
			m,
			selfPID,
			&fingerTable,
			&memberMap,
			sendMessage,
		)
		os.Exit(0)
	}()
}

// Periodically send out heartbeat messages with piggybacked membership map info.
func heartbeat(introducer bool) {
	for {
		if len(memberMap) == 1 && !introducer {
			log.Printf("[ORPHANED]: [SELFPID=%d] attempting to reconnect with introducer to find new peers.", selfPID)
			joinNetwork()
		}
		spec.CollectGarbage(
			selfPID,
			m,
			&memberMap,
			&suspicionMap,
			&fingerTable,
		)
		spec.RefreshMemberMap(selfIP, selfPID, &memberMap)
		message := fmt.Sprintf(
			"%d%s%s%s%d",
			spec.HEARTBEAT, delimiter,
			spec.EncodeMemberMap(&memberMap), delimiter,
			selfPID,
		)
		if logHeartbeats {
			log.Printf(
				"[HEARTBEAT] [selfPID=%d] [len(memberMap)=%d] [len(suspicionMap)=%d] ",
				selfPID,
				len(memberMap),
				len(suspicionMap),
			)
		}
		spec.Disseminate(
			message,
			m,
			selfPID,
			&fingerTable,
			&memberMap,
			sendMessage,
		)
		time.Sleep(time.Second * heartbeatInterval)
	}
}

func joinNetwork() {
	for {
		conn, err := net.Dial("udp", fmt.Sprintf("%s:%d", config.Introducer(), introducerPort))
		if err != nil {
			log.Printf("[ERROR] Unable to connect to introducer. Trying again in %d seconds.", joinAttemptInterval)
			time.Sleep(time.Second * joinAttemptInterval)
		} else {
			fmt.Fprintf(conn, "%d", spec.JOIN)
			conn.Close()
			return
		}
	}
}

func sendMessage(PID int, message string) {
	// Check to see if that PID is in our membership list
	target, ok := memberMap[PID]
	if !ok {
		log.Printf("sendMessage(): PID %d not in memberMap. Skipping.", PID)
		return
	}
	conn, err := net.Dial("udp", fmt.Sprintf("%s:%d", target.IP, heartbeatPort))
	if err != nil {
		log.Fatal(err)
	}
	fmt.Fprint(conn, message)
	conn.Close()
}

func getSelfIP() string {
	var ip string
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		log.Fatalf("Cannot get self IP")
		os.Exit(1)
	}
	for _, a := range addrs {
		if ipnet, ok := a.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				ip = ipnet.IP.String()
			}
		}
	}
	return ip
}

// RPCs
func serveRPCs() {
	log.Println("[RPC] serveRPCs launched")
	listener := new(Membership)
	rpc.Register(listener)
	rpc.HandleHTTP()
	l, e := net.Listen("tcp", ":"+strconv.Itoa(RPCPort))
	if e != nil {
		log.Fatal("listen error:", e)
	}
	go http.Serve(l, nil)
	log.Println("[RPC] serveRPCs done")
}

func (l *Membership) Self(_ int, reply *Self) error {
	if logHeartbeats {
		log.Println("[RPC] Membership.Self called")
	}
	*reply = Self{m, selfPID, memberMap, fingerTable, suspicionMap}
	return nil
}
