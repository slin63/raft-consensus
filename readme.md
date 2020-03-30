# <img src="./images/icon.png"/> Leeky Raft

An implementation of *Raft* as described in [this paper](https://pdos.csail.mit.edu/6.824/papers/raft-extended.pdf) with substantial help from the accompanying [Student's Guide to Raft](https://thesquareplanet.com/blog/students-guide-to-raft/). Leeky Raft serves as the consensus layer for [Chordish DeFiSh](https://github.com/slin63/chord-dfs).

## Setup

1. Start up server cluster
   1. `docker-compose build && docker-compose up --remove-orphans --scale worker=<num-workers>`
      1. For `num-workers`, 5 workers is recommended. You can scale to as many nodes as you want though, but your Raft might sink.
2. Send whatever entries you want with
   1. `CONFIG=$(pwd)/config.json LEADER=0 CLIENT=1 go run ./cmd/raft <your-entry>`

## Leeky Raft, Briefly

Leeky Raft was created to be used as the consensus layer inside of Chordish DeFiSh, a distributed file system. Leeky Raft ensures that all nodes within a network have up-to-date information about the collective state of the system, AKA what files are *in* the distributed filesystem, and what steps the system took to get there.

To explain the necessity for Leeky Raft requires us to explain a fundamental problem of distributed systems: *consensus*.

### The Problem with Consensus
#### Consensus, Briefly

You have *N* processes in a network. These processes communicate with one another by sending messages. Messages are packets of information. Any of those processes may fail at any time, becoming unresponsive to incoming messages and not sending out any messages themselves. 

Any responsive, non-failed process is called a *correct* process. The problem of consensus is described in the following three requirements:

- *Agreement*: You must try and get all correct processes to agree on a single value *v*, as proposed by any single process in the network. 
- *Validity:* All processes that return a value *r* must have *r* be a function of some input value from some other correct process
- *Termination*: All correct processes must eventually return a value *r*.

Consensus exists everywhere around us. In all the websites that we use, in our home devices, our laptops and phones, even in our basic human interactions. 

People implement and exercise extremely convoluted solutions to far more rigorous forms of the consensus problem every day. Think of a group of friends deciding where to eat as a consensus problem.

- **Formal prompt**: A set of *N* friends, *f*, where any element of *f* ∈ {Mike, Samantha, David, James} must agree on some place to eat, *r*, where *r* ∈ {Any of the 26,618 eateries in New York City.}

- **Example**: Mike wants falafel cart. But Samantha hates falafel because of an irrational hatred for chickpeas. She proposes dollar pizza. James hates dollar pizza, because it gave him diarrhea last week, and the week before. Mike hears everyone's complaints and suggests Chinese food, which has neither chickpeas or pizza in it. David catches up to the group, after lagging behind while petting a dog, and proposes dollar pizza. Everyone explains to David why they can't do that.

  Suddenly, New York is completely shut down for a pandemic. Nobody eats anywhere. 

  12 years later, having survived the sudden apocalypse, all friends in *f*, except for Mike (RIP), who is succeeded by his 11 year old son, Muz∆¥løek, born of a world rapt in horror and uncertainty, rejoin and decide on a value *r*: the pile of burnt out cars on Parkside & Bedford in Brooklyn. They're going to have stone soup.

This is a nightmare. 

Not because of the post-apocalyptic nature of this scenario. Also not because they're going to the pile of burnt out cars on Parkside & Bedford to have stone soup, which is really the worst thing on the menu there. 

It's a nightmare because out of the *N* processes, any single one can propose a value. But now these processes also have the ability to *veto* and remove values from the pool of choices. Also, there are arbitrary *pools of choices*, as opposed to the binary consensus problem we described earlier. Not to mention that these processes can lag behind and re-propose previously vetoed values, wasting network time. Then, without warning, we see that the system is one that can spontaneously fail. 

Finally, despite all the setbacks and an incredible amount of network downtime, all *correct* processes rejoin and decide on a value. Also, in a turn of events completely irrelevant to our consensus problem, the *Mike* process managed to spawn a child process, *Muz∆¥løek*, who is allowed to participate in the network. 

This is consensus.

So now that you have a feel for what the consensus problem is and how hairy it can become, let's talk about why it's so interesting.

#### Consensus is Fundamental

Consensus serves as the basis for countless problems inside distributed systems. I'll just list a few here.

- *Reliable Multicast*: Guarantee that all processes within a network receive the same update in the same order
- *Membership/Failure Detection*: Having processes maintain a local list of all other processes in a network, updating on membership changes like processes leaving or failing
- *Leader Election*: Agree on a single leader process and notify the entire network of the new leader 
- *Mutual Exclusion/Distributed Locking*: Allow only one process at a time to access a critical resource, such as a file

Any protocol for solving the basic problem of consensus also, by extension, can be leveraged to solve all of the above problems as well. Isn't that amazing? 

With so many great minds in the field of distributed computing, consensus must have so many great and proven solutions! Aren't you excited?

#### Consensus is Impossible

Surprise! Formally proven in [Impossibility of Distributed Consensus with One Faulty Process (FLP)](https://groups.csail.mit.edu/tds/papers/Lynch/jacm85.pdf), consensus, although possible to achieve in *synchronous* systems, is impossible in an *asynchronous* system. The paper describes consensus as the following:

> an asynchronous system of [unreliable] processes . . . [trying] to agree on a binary value (0 or 1). Every protocol for this problem has the possibility of nontermination, even with only one faulty process.

To understand the asserted point here, it is important to distinguish some key differences between synchronous and asynchronous system models. 

In a *synchronous system model*, there is a known upper bound on message delivery time between correct processes. If a process hasn't responded in that amount of time, it's failed. An example of a synchronous system might be a single supercomputer doing a lot of calculations. 

In an *asynchronous system model*, messages can be delayed indefinitely, taking anywhere from 1ms to a year to never arriving to their destination. An example of an asynchronous system might be several computers, working together to do a larger calculation, assigning work and returning results over the network. 99% of real world over-network applications fall into this category.

Imagine a synchronous system trying to achieve consensus. If a process takes too long to respond to something, because of the upper bound on message delivery time, we know that that process is no longer *correct*, and that we don't have to wait on it for a response. Eventually all the *correct* nodes put in their votes, and consensus is achieved.

Now imagine an asynchronous system. If a process takes too long to respond to something, because *there is no* upper bound on message delivery time, how do we know that this process is guaranteed to be *no longer correct*? The answer: it's impossible. Failed processes and processes that are very slow to respond are indistinguishable in an asynchronous system. 

Sure, you could implement a simple time-based failure detection protocol like I did with [Chord-ish](https://github.com/slin63/chord-failure-detector), but another problem arises. If we wrongly mark the slow process as failed and proceed with voting, we violate termination. 

Remember termination from our earlier definition of requirements for consensus? Termination requires that all correct processes must eventually return a value *r*. Although our wrongly marked process is extremely slow to respond, it is still a correct process. Coming to a decision without considering that processes' output is a violation of termination.

This is just one of *many* ways that an asynchronous system can fail to come to true consensus.

But it's okay, don't worry.

#### Most Protocols Are Good Enough

Although solving the formal problem of consensus in asynchronous systems is impossible, there exist plenty of protocols that guarantee *safety* with high probability. Safety here meaning that the system will never return an incorrect value. While developing and implementing consensus protocols is still no small feat, many protocols exist in the wild and are attached to names you might find [very familiar](https://en.wikipedia.org/wiki/Consensus_(computer_science)#Some_consensus_protocols).

One of these protocols is Raft. In the next section I'll talk about Raft, my implementation of it, and its utility as the consensus layer for Chord DeFiSh.

### Enter: Raft

[*Raft*](https://en.wikipedia.org/wiki/Raft_(computer_science)) a consensus algorithm that is formally proven safe. 

1. What is consensus? Why's it cool?
   1. asynchronous vs synchronous systems
   2. real life is asynchronous
2. Why'd I use Raft?
3. How did I implement Raft?
   1. Integrating with the Membership Layer
   2. TL;DR explanation of Raft's algorithm
4. Why'd I use Docker? be brief (expand on further in future overview post)
5. Why was it a pain in the ass?
   1. Elections
   2. So many livelocks
6. Final result & feature set

## Running "tests"*

1. `CONFIG=$(pwd)/config.json go test -v ./internal/...`

\* These tests don't work.

