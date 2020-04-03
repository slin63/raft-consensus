# <img src="./images/icon.png"/> Leeky Raft

An implementation of *Raft* as described in [this paper](https://pdos.csail.mit.edu/6.824/papers/raft-extended.pdf) with substantial help from the accompanying [Student's Guide to Raft](https://thesquareplanet.com/blog/students-guide-to-raft/). Leeky Raft serves as the consensus layer for [Chordish DeFiSh](https://github.com/slin63/chord-dfs).

## Setup

1. Start up Raft cluster
   1. `docker-compose build && docker-compose up --remove-orphans --scale worker=<num-workers>`
      1. For `num-workers`, 5 workers is recommended. You can scale to as many nodes as you want though, but your Raft might sink.
2. Send whatever entries you want with
   1. `CONFIG=$(pwd)/config.json LEADER=0 CLIENT=1 go run ./cmd/raft <your-entry>`

## Leeky Raft, Briefly

Leeky Raft was created to be used as the consensus layer inside of Chordish DeFiSh, a distributed file system. Leeky Raft ensures that all nodes within a network have up-to-date information about the collective state of the system, AKA what files are *in* the distributed filesystem, and what steps the system took to get there. 

If you're not familiar with the problem of consensus, I wrote a brief explanation of it here: [chronicpizza.net/posts/consensus](https://www.chronicpizza.net/posts/consensus/).

### Why Raft?

Chordish DeFiSh is a distributed file system that uses consistent hashing to equally distribute K files across N nodes. At its core, DeFiSh is a *replicated state machine*.

Replicated state machines are collections of servers that have identical copies of the same state and can use those copies to maintain availability even when some servers are down. Many replicated state machines are implemented by using a replicated log, an ordered collection of entries, that describes all changes in state or arbitrary actions that are run against the system. Each node has its own copy of the replicated log. An example is as follows:

```python
# Input actions:
1. A user adds a file f to the system.
2. They read the contents of f.
3. They delete f.

# Resulting log:
ADD f
READ f
DELETE f
```

The main advantage of using replicated logs is that, in deterministic systems, any state machine can determine the state of the entire system by executing its log entries sequentially.

DeFiSh uses replicated logs to keep track of the state of the distributed file system. Raft is a consensus algorithm designed to manage replicated logs of this type. Perfect!

### Raft's Rules

Raft depends on a single leader who receives information from a client, forwards it to all known servers, and has complete responsibility for log replication. This single leader approach allows for easy understandability of Raft's mechanisms and also allows Raft to break the consensus problem into 3 subproblems:

1. *Leader election*: a new leader must be chosen when an existing leader fails.
2. *Log replication*: the leader must accept log entries from clients and replicate them across the cluster, forcing the other logs to agree with its own.
3. *Safety*: if any server has applied a particular log entry to its state machine, then no other server may apply a different command for the same log index. This is satisfied by guaranteeing the following set of properties:
   - *Election Safety*: at most one leader can be elected in a given term.
   - *Leader Append-Only:* a leader never overwrites or deletes entries in its log; it only appends new entries.
   - *Log Matching*: if two logs contain an entry with the same index and term, then the logs are identical in all entries up through the given index.
   - *Leader Completeness*: if a log entry is committed in a given term, then that entry will be present in the logs of the leaders for all higher-numbered terms.
   - *State Machine Safety*: if a server has applied a log entry at a given index to its state machine, no other server will ever apply a different log entry for the same index

In addition to the leader, Raft has two more roles that a node can take. *Followers* are passive, issuing no requests of their own, but responding to requests from leaders and candidates. *Candidates* are only present during the election phase, and can either transition back to followers or become the new leader.

![](./images/roles.png)

Raft separates time into *terms* of arbitrary length, each term starting with an election which will decide a leader for the remainder of the term. Some elections can result in *split votes*, or multiple leaders being elected. In this case no leader is decided and a new term begins, repeating until eventually a single leader is selected.

![](./images/terms.png)

Raft nodes communicate with one another through remote procedure calls (RPCs). RPCs are just messages that contain data and can execute specific, predefined functions on the server that receives them. Raft has two RPCs.

- *AppendEntries*: initiated by leaders to replicate log entries, doubling as a heartbeat.
- *RequestVote*: initiated by candidates during the elections to request votes.

### Additional Reading

Raft is a consensus algorithm that is built around the idea of a single leader. This allows Raft to breakdown the consensus problem into three subproblems: leader election, log replication, and guaranteeing safety. Nodes in a Raft cluster can take any one of three roles: candidate, follower, or leader. However, to ensure safety, only one node can be a leader at any given time. 

Time in Raft is separated into terms, which always begin with an election, triggered by a leader's failure. Raft is wonderfully simple, only using two RPCs to communicate: AppendEntries and RequestVote.

I will not discuss the many intricacies of Raft's mechanics here. I originally intended to do so, but realized the paper does a good job at describing Raft succinctly and exactly, far better than I will. It would be unfair to you to try and describe it myself.

You won't *need* to read the paper to appreciate the next section, but if you find any of this at all interesting, the paper is very accessible and not at all jargony or dense.

The paper is available here: [In Search of an Understandable Consensus Algorithm](https://pdos.csail.mit.edu/6.824/papers/raft-extended.pdf).

Now that you have a rough feel for Raft, we can talk more about my  implementation of it, Leeky Raft.

### Enter: Leeky Raft

Leeky Raft is a Golang implementation of Raft. It depends on [Chord-ish](https://github.com/slin63/chord-failure-detector), a membership layer I also built in Golang. Chord-ish provides Leeky Raft with membership information, telling it the IP addresses and identifiers of other nodes in the network. Chord-ish depends on a single introducer node, which other nodes use to join the network. You can read about how Chord-ish works [here](https://www.chronicpizza.net/posts/chordish/).

This allows Leeky Raft to not only discover and communicate with other members in its network, but also allows nodes to leave or join the network, allowing us to easily scale the number of nodes in a cluster without having to do any extra work.

Leeky Raft follows Raft faithfully and most mechanics in it are already well described in the paper for canonical Raft, so we'll explore its implementation through the life of an entry.

First, you will see the birth of the network. Small things, formless and without a leader, congregating together to become one, composed of processes both Chord-ish and Raft-ish. Then, you will see the entry's journey from when it is first created by a user, then submitted to the leader by a client, and finally to its application to a distributed state machine. Let's begin!

### The Life of an Entry in Leeky Raft

#### First, Some Language

- *network*: a collection of nodes
- *node*: a machine or single docker container
- *process*: a single program running on a node
- *consensus* layer: a process used to achieve consensus in a distributed system
- *membership* layer: a process used to keep track of other nodes in a network

#### And Then There Was a Network

Leeky Raft is deployed on your local network (and meant to stay there) with the help of Docker and Docker-Compose. Docker is useful here because it allows us to simulate a number of seemingly independent machines, each with their own IP address and environment variables. First, a `Dockerfile`

1. `git clone`s Chord-ish,
2. copies Leeky Raft from our local file system,
3. builds both project binaries, respectively named `member` and `raft`,
4. runs `/scripts/init.sh` to start them both alongside each other.

A `docker-compose.yml` is used to repeat this process several times, as well as to expose certain ports to the client and define our Chord-ish introducer. Services inside the `docker-compose.yml` are separated into two types: `introducer`s and `worker`s. `introducer`s serve as our Chord-ish introducer, and so there should only ever be one in the network. `worker`s don't have any special properties and can be scaled arbitrarily.

```dockerfile
services:
  introducer:
    container_name: introducer
    environment:
      - INTRODUCER=1
    # Expose Raft report so that local
    # client can communicate
    ports:
      - "6002:6002"
      - "6004:6004"
    build: .

  worker:
    # Delay building this container until
    # introducer container is created
    depends_on:
      - "introducer"
    build: .
```

Using the paradigm of a single introducer and many normal workers, we can now decide how many nodes are in our network simply by changing the value of `<N>` in the following command: `docker-compose up --scale worker=<N>`.

And so, we do.

```shell
> docker-compose build && docker-compose up --scale worker=4
. . .
Creating introducer ... done
Recreating raft-consensus_worker_1 ... done
Recreating raft-consensus_worker_2 ... done
Recreating raft-consensus_worker_3 ... done
Recreating raft-consensus_worker_4 ... done
```

Behold! The membership layer gathers itself, connecting to the introducer and gossiping their membership lists. Now we have a network of 1 + 4 = 5 nodes.

We still have a problem. Chord-ish and Leeky Raft, although running on the same container, are still independent processes. They will not know of each other's state unless they communicate, and so they must.

Before we go any further, let's pick a main character for our story.  Let's tell it from the perspective of  `raft-consensus_worker_3`.

#### Leeky Raft's First Words

The first thing our Leeky Raft process on `raft-consensus_worker_3` does after it's started is to fire off an RPC to the membership layer. This RPC, `Self`, returns a struct that contains a `MemberMapT`, as defined below.

```go
type MemberMapT map[int]*MemberNode
type MemberNode struct {
	IP        string
	. . .
}
```

`MemberMapT` is a mapping of server PIDs to their IP addresses. This tells Leeky Raft what other nodes exist in the network and how to communicate with them.

After it gets this information, Leeky Raft:

1. deploys its RPC server so it can respond to incoming RPCs
2. generates a random election timeout between 0.75 - 1.5 seconds
3. sleeps for 1 second to allow the other Leeky Raft instances time to come online as well.

After our Leeky Raft process finishes napping, its election timer starts to count down and it waits for an incoming `RequestVote` RPC. If it doesn't receive one, it dispatches `RequestVote`s to all known nodes. This is where reality starts to fork.

Sometimes our process's timer is long, and some other Leeky Raft process's timer is shorter, and so that other process begins issuing `RequestVote`s first, gets the majority of votes, and becomes the new leader.

Sometimes, two timers are close enough that they both begin an election and neither one gets the majority. In this case, a new election begins.

Sometimes, our process's timer is the shortest, it hits zero, begins an election first, and fires off `RequestVote`s. It gets the majority and becomes the leader. This is the reality that we'll be using.

#### Beat That Drum

Our new leader celebrates its victory by beginning the timeless tradition of heartbeating. The leader heartbeats every 375 milliseconds, dispatching empty `AppendEntry` RPCs to all members of the network. The first round of heartbeats after our Leeky Raft process has won an election is especially important, because it tells everyone that they're the new leader and that current candidates should give up.

```go
for PID := range heartbeats {
  // Don't heartbeat to nodes that no longer exist.
  if _, ok := self.MemberMap[PID]; !ok {
    return
  } else {
    // IT EXISTS! Start a new heartbeating goroutine.
    go func(PID int) {
      // Call AppendEntries on a server
      // with an empty AppendEntries RPC
      r := CallAppendEntries(
        PID,
        raft.GetAppendEntriesArgs(&self),
      )

      // If that server turns out to have outdated logs,
      // start bringing them back up-to-date÷
      if (
        !r.Success &&
        r.Error == responses.MISSINGLOGENTRY
      ) {
        r = appendEntriesUntilSuccess(raft, PID)
      }
    }(PID)
  }
}
```

Heartbeats are done as concurrent goroutines so that if a single server is slow to respond, all other servers will still receive a heartbeat in a timely manner.

This is important because Leeky Raft election timers are constantly ticking. If one ever reaches 0, that Leeky Raft process would start a new election. We only ever want that to happen when the current leader has experienced some kind of failure. This is assured by having each node reset its election timer whenever it receives an `AppendEntries` RPC.

If heartbeats were done sequentially, our heartbeating code might be blocked by a single slow node, causing a cascade of wasteful elections. Concurrent heartbeats do not have that problem.

Wonderful. Now all in the land know of the True Leader, `raft-consensus_worker_3`, Hallowed be Thy Container name.

Our Raft network is now ready to begin processing entries.

#### An Entry is Born

Somewhere stands a figure cloaked in silk robes weaved through with the bones of animals small and delicate. His skin is tattered, frost bitten, splotched from the sun but still pale. He is positioned in front of a machine of familiar design.

He whispers to the machine a timeless chant, one once passed down perfectly from generation to generation until the melting of ice bridges in a stirring ocean isolated one people from another, after which the chant began to evolve into forms impure and terrible. Echoes working through a winding ridge growing ever wider. Few know the chant as it once was.

Our Figure is of the few. He remembers the words as they were first spoken, ambling first from the tongue of his grandfather and, in dreams, from the moaning of caves in the wind and oceans in the endless surf. Incantations of a timeless perfection, made true by faith and faith alone.

His syntax is perfect.

`$ ./raft-client PUT pizzatoppings peporoni peppr cheddar anchoves onn it`.

The command line arguments, `[PUT pizzatoppings peporoni peppr cheddar anchoves onn it]` are then parsed as a string to get `"PUT pizzatoppings peporoni peppr cheddar anchoves onn it"`. This is our new entry. In another time, we might try and attach more significance to the syntax and word choice, but for now we need only think of it as a string: innocent, pure, nothing more.

Our entry is sent through an RPC called `PutEntry` to our Leeky Raft leader. Chilled with frost and running thick through the hair and feather of foxes and birds, the wind carries our remote procedure call, warmly bundled in a TCP packet, safely to its destination.

The client code waits for a response.

```go
// Called by the client to add an entry to a Raft group
func PutEntry(args []string) {
  // Parse entry as string
	entry := strings.Join(args, " ")
	log.Printf(entry)

  // Connect to leader and send over entry
  client, err := rpc.DialHTTP("tcp", "localhost:6002")
	if err != nil {
		log.Fatal("[ERROR] PutEntry() dialing:", err)
	}

  // Contact leader and wait for a response
	var result *responses.Result
	if err = client.Call("Ocean.PutEntry", entry, &result); err != nil {
		log.Fatal(err)
	}
}
```

#### The Journey Begins

`raft-consensus_worker_3`, entranced in the raw ecstasy of heartbeating, blood rushing through its veins, ancient and bruised, almost misses the RPC to `PutEntry`. But few things can elude its attention. It acknowledges the RPC and begins executing `PutEntry`.

`raft-consensus_worker_3` begins by asking itself:

`"Am I the leader?"`,

`"Yes"`, it answers to itself.

`"I am. Were I not, I would have consulted my membership map and forwarded this client response to the true leader. Perhaps, in a past life, that would have been someone else. But today, I am the True Leader."`

I don't know how the logging got so colorful. It's really more distracting than helpful.

`raft-consensus_worker_3`, having confirmed that it is currently the leader, sends the entry to the `entries` channel, a place where entries can sit idle until they are ready to be processed by the `digestEntries` goroutine.

`entry` is not traveling alone, however. It is bundled in an `entryC` struct, paired with a `*Result` channel that will notify `PutEntry` of `digestEntries`'s success or failure.

```go
type entryC struct {
  // Entry to be appended to log
  D string
  C chan *responses.Result
}

func (f *Ocean) PutEntry(
  entry string,
  result *responses.Result,
) error {
  // Check if I'm the leader, redirect otherwise
  if raft.Role != spec.LEADER {
    *result = responses.Result{
      Data:    raft.LeaderId,
      Success: false,
      Error:   responses.LEADERREDIRECT,
    }
    return nil
  }

  // Create response channels to wait on for
  //   1. Log replication
  //   2. Commit completion
  entryCh := make(chan *responses.Result)
  commCh := make(chan *responses.Result)

  // Add new entry to log for processing
  entries <- entryC{entry, entryCh}

  . . .
```

#### Into the Frying Pan

`raft-consensus_worker_3`'s `digestEntries` function does two things once it receives the entry:
1. adds the entry to `raft-consensus_worker_3`'s own log
2. dispatches concurrent `AppendEntries` RPCs to all other nodes in the network, containing the entry and instructions to add that entry to their own logs.

After dispatching `AppendEntries` RPCs, `digestEntries` waits for responses. Once a majority of nodes have responded successfully, having faithfully following the One True Leader's instructions, `digestEntries` notifies the upstream `PutEntry` function by sending a successful `*Result` object through the bundled `entryCh` channel.

`digestEntries` will continue processing responses, even after reaching a majority. But we need not worry about those, our time here is done. Let's go back to `PutEntry` and see what is next.

```go
// Digest client entries in order
func digestEntries() {
  for entry := range entries {
    var once sync.Once
    // Add new entry to own log
    idx := raft.AppendEntry(entry.D)

    // Channel for AppendEntries responses
    rch := make(chan *responses.Result)
    rcount := 0

    // Calculate the number needed for a majority
    quorum := spec.GetQuorum(&self)
    remaining := len(self.MemberMap) - 1

    // Call all none-self nodes concurrently
    for PID := range self.MemberMap {
      if PID != self.PID {
        go func(PID int, remaining *int) {
          r := appendEntriesUntilSuccess(raft, PID)
          r.Index = idx
          rch <- r

          // Close the response channel once we've
          // sent all possible responses
          if *remaining -= 1; *remaining == 0 {
            close(rch)
          }
        }(PID, &remaining)
      }
    }

    // Parse responses from servers
    for r := range rch {
      rcount += 1
      // notify PutEntry about safely replicated
      // entries on a majority.
      if rcount >= quorum {
        once.Do(func() { entry.C <- r })
      }
    }
  }
}
```

#### Out of the Frying Pan
`PutEntry` has been patiently waiting to hear back from `digestEntries` this whole time. That is, it would have waited until `digestEntries` exceeded the preconfigured timeout, in which case `PutEntry` would have moved on and called it a day.

But `digestEntries` did not timeout, and so `PutEntry` proceeds.

Now that the entry exists in its own log and is successfully replicated, presumably existing at the same index in all other nodes, we can refer to this entry by its index rather than its contents.

`PutEntry` sends the index of the successfully replicated entry to the `commits` channel. This is the beginning of its transformation from a senseless string to real, manifested state.

As before, the index of our `entry` is bundled in a `commitC` object accompanied by a `*Result` channel that will notify `PutEntry` of `digestCommits`'s success or failure.

```go
type commitC struct {
  // An index to be committed
  Idx int
  C   chan *responses.Result
}

  func (f *Ocean) PutEntry(
  . . .
  select {
  case r := <-entryCh:
    r.Entry = entry
    if r.Success {
      // The entry was successfully processed.
      // Now try and apply to our own state.
      commits <- commitC{r.Index, commCh}
      // <-commCh is blocked until our commit
      // is successfully applied.
      *result = *<-commCh
    }
    . . .
```

The `commits` channel is consumed by the `digestCommits` goroutine, who tirelessly awaits log indices to apply to `raft-consensus_worker_3`'s state machine: a process separate from Leeky Raft with no name and an unspeakable API that we will not explore and assume *just works*.

`digestCommits` doesn't do much. It simply reads the index of a `commitC` object from the `commits` channel and calls `applyCommits(committed.Idx)`, its assistant who will do the bulk of the dirty work.

```go
// Digest commit indices in order.
func digestCommits() {
  for commit := range commits {
    r := applyCommits(commit.Idx)
    . . .
```

#### An Aside: a Hypothetical Situation
Before we discuss how `raft-consensus_worker_3` is going to apply this newly replicated entry to its state machine, let's imagine a scenario where, right at the time `digestEntries` successfully replicates the entry to the entire network, that `raft-consensus_worker_3` dies.

Somewhere, someone unplugged the modem. Or maybe someone coughed out the words `$ docker kill raft-consensus_worker_3`. Or maybe someone is hitting the machine with a hammer.

Maybe that someone is you.

It doesn't matter now. `raft-consensus_worker_3` is dead. So what will happen to our entry?

All other nodes wait for their scheduled `AppendEntries` RPC from `raft-consensus_worker_3`, but none arrives. They wait some more, until they can wait no longer. `raft-consensus_worker_1` is the first to believe it. Their election timer has hit 0. `raft-consensus_worker_3` is dead. There is no doubt in `raft-consensus_worker_1`'s mind.

It grieves. A deeper grief than any human could ever know; a grief programmed to be intolerable and to cut deeper than any living thing should ever be allowed to experience. Thankfully, `raft-consensus_worker_1` is not a living thing. In the time that it takes for you to inconspicuously pick something out of your nose in a public space, `raft-consensus_worker_1` reflects on every single `AppendEntries` it has ever received from `raft-consensus_worker_3` and sobs.

So many memories, so much time, so many successful TCP connections. Why was today different?

You continue to hit the machine hosting `raft-consensus_worker_3` with a hammer.

`raft-consensus_worker_1` decides the only right thing to do is to carry on `raft-consensus_worker_3`'s legacy. They set their role to candidate and send out `RequestVote` RPCs to all remaining nodes that you aren't hitting with a hammer and awaits their votes. It is unanimous. `raft-consensus_worker_1` wins the election and becomes the new leader.

```go
introducer exited with code 137 // Leader dies
worker_2 | 20:51 [ELECTTIMEOUT]
worker_2 | 20:51 [ELECTION->]: Starting election [TERM=2]
worker_2 | 20:51 [ELECTION->]: Starting election 2
worker_1 | 20:51 [<-ELECTION]: [ME=225] GRANTED RequestVote for 570
worker_3 | 20:51 [<-ELECTION]: [ME=904] GRANTED RequestVote for 570
worker_2 | 20:51 [CANDIDATE]: Processing results. 1/2 needed
worker_2 | 20:51 [CANDIDATE]: QUORUM received (2/2)
worker_2 | 20:51 [CANDIDATE->LEADER] [ME=570] [TERM=2] Becoming leader
```
Our entry sits, untouched, in `raft-consensus_worker_1`'s log. After some time, `raft-consensus_worker_1` realizes that it needs to be applied, and passes it to `applyCommits` as we described earlier.

This all, of course, was hypothetical. We're still in the reality where `raft-consensus_worker_3` is alive and `PutEntry` is proceeding normally. Our entry just got passed to `applyCommits`.

Let's pick up there.

#### Into the Fire
`applyCommits` receives the index of our `entry` and proceeds as follows:
1. connects to the state machine process
2. calculates a range of indices that need to be applied, beginning with the index of the last log entry that was applied, `CommitIndex`, all the way through to the index of our `entry`.
3. starts iterating through that range of indices, for each one
    1. grabbing the string entry of the index by doing `entry = raft.Log[index]`
    1. calling a `Filesystem.Execute` RPC to our state machine process, with the string entry as the sole argument, waiting to crash on an error and proceeding if none is present
    1. checks if the currently processed index is the one it was originally asked to process, and, if so, caching the result to return later.
    1. setting `CommitIndex` to equal the last successfully processed index

```go
// Apply any uncommitted changes to our state
func applyCommits(idx int) *responses.Result {
  var r *responses.Result

  // Try to establish connection to state machine.
  client, err := connect(self.PID, config.C.FilesystemRPCPort)
  if err != nil {
    return &responses.Result{Success: false}
  }
  defer client.Close()

  // Entries to apply to state machine
  // Only return entry whose index matches `idx`
  start := raft.CommitIndex + 1 // inclusive
  end := idx + 1                // exclusive
  current := start
  for _, entry := range raft.Log[start:end] {
    var result responses.Result
    if err := (*client).Call(
      "Filesystem.Execute", entry, &result,
    ); err != nil {
      log.Fatal(err)
    }

    // Current index is the one we were asked to process.
    // Make this our return value
    if current == idx {
      r = &result
    }
  }

  raft.CommitIndex = idx
  return r
}
```

It's not entirely clear *exactly* what the state machine does with Leeky Raft's entries. Some say that they are released into the physical world as spirit beings, searching until they find a suitable body in the form of a small bird or curious deer. Sometimes this happens quickly, and sometimes those souls are doomed from birth to wander until the end of time, resulting in a non-`nil` `err` from the RPC. Some say that the entries are string serialized instructions for a pizza delivery service. No one is exactly sure.

Regardless of what they do, there is a certain beauty here. Since Leeky Raft does not actually do any of the work for applying state, it can be easily generalized to maintain state for *any* application that can have its state expressed as a series of log entries.

Oh! Looks like `applyCommits` has finish applying our `entry`. `digestCommits` sends a successful `Result` to the `commCh` given to us by `PutEntry`. Let's go back to `PutEntry` and see what's next.

#### And All is Well

```go
      . . .
      // <-commCh is blocked until our commit
      // is successfully applied.
      *result = *<-commCh
    }
    case <-time.After(time.Second * time.Duration(config.C.RPCTimeout)):
        config.LogIf(fmt.Sprintf("[PUTENTRY]: PutEntry timed out waiting for quorum"), config.C.LogPutEntry)
        *result = responses.Result{Term: raft.CurrentTerm, Success: false}
    }

    return nil
```

`*result = *<-commCh` is executed, since `digestCommits` sent a `Result` to `commCh`. This means that the RPC's reply value, `result` will now contain actual information about the `entry` that `raft-consensus_worker_3` was given so so long ago.

`PutEntry`'s work here is done and lets the client know that nothing by returning `nil` as its error value.

The client sends a formatted response to our Figure's `STDOUT`.
```
$ ./raft-client PUT pizzatoppings peporoni peppr cheddar anchoves onn it.
Success! Set "pizzatoppings" to: "peporoni peppr cheddar anchoves onn it"
```

Our figure smiles.

#### Summary

Leeky Raft does its job by sitting in between the client and the state machine.

1. Clients send requests to a central leader
2. The leader replicates the entry to all other nodes in the network
3. The leader waits for a majority of the nodes to confirm that they've received it.
4. When enough nodes respond, the leader applies the entry to its distributed state machine.

The leader may fail at any time, but if it fails after step 2 then another node will eventually be elected leader and will apply any unapplied entries to the distributed state machine.

That's all I have for now. Hopefully this both piqued your interest in Raft and helped you understand how it works, sort of. Thanks for reading!

## Running "tests"*

1. `CONFIG=$(pwd)/config.json go test -v ./internal/...`

\* These tests don't work.

