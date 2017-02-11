/*
Package dinghy is a leader election mechanism that follows
the raft protocol https://raft.github.io/ using http as the transport,
up until the use of replicated log. For a more complete raft implementation
use https://godoc.org/github.com/coreos/etcd/raft or https://github.com/hashicorp/raft

Leader Election

The process begins with all nodes in FOLLOWER state and waiting for an election timeout.
This timeout is recommended to be randomized between 150ms and 300ms. In order to reduce the
amount of traffic this will be increased to ElectionTickRange.

After the election timeout, the FOLLOWER becomes a CANDIDATE and begins an election term.
It starts by voting for itself and then sends out a RequestVote to all other nodes.

If the receiving nodes haven't voted yet, they will then vote for the candidate with a
successful request. The current node will reset it's election timeout and when a
candidate has a majority vote it will become a LEADER.

At this point the LEADER will begin sending an AppendEntries request to all other
nodes at a rate specified by the heartbeat timeout. The heartbeat timeout should be shorter
than the election timeout, preferably by a factor of 10.
Followers respond to the AppendEntries, and this term will continue until a follower
stops receiving a heartbeat and becomes a candidate.

There is a case where two nodes can be come candidates at the same time, which is
referred to a split vote. In this case two nodes will start an election for the same term
and each reaches a single follower node before the other. In this case each candidate will
have two votes with no more available for the term and no majority. A new election will happen
and finally a candidate will become a LEADER. This scenario can happen with an even number of
nodes.
*/
package dinghy
