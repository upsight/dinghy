package dinghy

import (
	"errors"
	"fmt"
	"net/http"
	"sync"
)

var (
	// DefaultOnLeader is a no op function to execute when a node becomes a leader.
	DefaultOnLeader = func() error { return nil }
	// DefaultOnFollower is a no op function to execute when a node becomes a follower.
	DefaultOnFollower = func() error { return nil }
	// DefaultRoutePrefix is what is prefixed for the dinghy routes. (/dinghy)
	DefaultRoutePrefix = "/dinghy"
	// ErrTooFewVotes happens on a RequestVote when the candidate receives less than the
	// majority of votes.
	ErrTooFewVotes = errors.New("too few votes")
	// ErrNewElectionTerm if during RequestVote there is a higher term found.
	ErrNewElectionTerm = errors.New("newer election term")
	// ErrLeader is returned when an operation can't be completed on a
	// leader node.
	ErrLeader = errors.New("node is the leader")
	// ErrNotLeader is returned when an operation can't be completed on a
	// follower or candidate node.
	ErrNotLeader = errors.New("node is not the leader")
)

// Dinghy manages the raft FSM and executes OnLeader and OnFollower events.
type Dinghy struct {
	client *http.Client
	logger Logger
	mu     *sync.Mutex
	// routePrefix will be prefixed to all handler routes. This should start with /route.
	routePrefix string
	stopChan    chan struct{}
	// Addr is a host:port for the current node.
	Addr string
	// Nodes is a list of all nodes for consensus.
	Nodes []string
	// OnLeader is an optional function to execute when becoming a leader.
	OnLeader func() error
	// OnFollower is an optional function to execute when becoming a follower.
	OnFollower func() error
	// State for holding the raft state.
	State *State
}

// ApplyFunc is for on leader and on follower events.
type ApplyFunc func() error

// New initializes a new dinghy. Start is required to be run to
// begin leader election.
func New(addr string, nodes []string, onLeader, onFollower ApplyFunc, l Logger, eMS, hMS int) (*Dinghy, error) {
	l.Println("addr:", addr, "nodes:", nodes)

	if onLeader == nil {
		onLeader = DefaultOnLeader
	}
	if onFollower == nil {
		onFollower = DefaultOnFollower
	}
	c := &http.Client{}

	id, err := hashToInt(addr, len(nodes)*1000)
	if err != nil {
		return nil, err
	}
	id++
	d := &Dinghy{
		client:      c,
		logger:      l,
		mu:          &sync.Mutex{},
		routePrefix: DefaultRoutePrefix,
		stopChan:    make(chan struct{}),
		Addr:        addr,
		Nodes:       nodes,
		OnLeader:    onLeader,
		OnFollower:  onFollower,
		State:       NewState(id, eMS, hMS),
	}
	l.Printf("%+v", d)
	return d, nil
}

// Stop will stop any running event loop.
func (d *Dinghy) Stop() error {
	d.logger.Println("stopping event loop")
	// exit any state running and the main event fsm.
	for i := 0; i < 2; i++ {
		d.stopChan <- struct{}{}
	}
	return nil
}

// Start begins the leader election process.
func (d *Dinghy) Start() error {
	d.logger.Println("starting event loop")
	for {
		d.logger.Println(d.State)
		select {
		case <-d.stopChan:
			d.logger.Println("stopping event loop")
			return nil
		default:
		}
		switch d.State.State() {
		case StateFollower:
			d.follower()
		case StateCandidate:
			d.candidate()
		case StateLeader:
			d.leader()
		default:
			return fmt.Errorf("unknown state %d", d.State.State())
		}
	}
}

// follower will wait for an AppendEntries from the leader and on expiration will begin
// the process of leader election with a RequestVote.
func (d *Dinghy) follower() {
	d.logger.Println("entering follower state, leader id", d.State.LeaderID())
	err := d.OnFollower()
	if err != nil {
		d.logger.Errorln("executing OnFollower", err)
	}
LOOP:
	for {
		select {
		case <-d.stopChan:
			return
		case newState := <-d.State.StateChanged():
			if newState == StateFollower {
				continue
			}
			d.logger.Println("follower state changed to", d.State.StateString(newState))
			return
		case <-d.State.HeartbeatReset():
			d.logger.Println("heartbeat reset")
			continue LOOP
		case aer := <-d.State.AppendEntriesEvent():
			d.logger.Println(d.State.StateString(d.State.State()), "got AppendEntries from leader", aer)
			continue LOOP
		case h := <-d.State.HeartbeatTickRandom():
			// https://raft.github.io/raft.pdf
			// If a follower receives no communication over a period of time
			// called the election timeout, then it assumes there is no viable
			// leader and begins an election to choose a new leader.
			// To begin an election, a follower increments its current
			// term and transitions to candidate state.
			d.logger.Println("follower heartbeat timeout, transitioning to candidate", h)
			d.State.VotedFor(NoVote)
			d.State.LeaderID(UnknownLeaderID)
			d.State.Term(d.State.Term() + 1)
			d.State.State(StateCandidate)
			return
		}
	}
}

// candidate is for when in StateCandidate. The loop will
// attempt an election repeatedly until it receives events.
// https://raft.github.io/raft.pdf
// A candidate continues in
// this state until one of three things happens: (a) it wins the
// election, (b) another server establishes itself as leader, or
// (c) a period of time goes by with no winner.
func (d *Dinghy) candidate() {
	d.logger.Println("entering candidate state")
	go func() {
		d.logger.Println("requesting vote")
		currentTerm, err := d.RequestVoteRequest()
		if err != nil {
			d.logger.Errorln("executing RequestVoteRequest", err)
			switch err {
			case ErrNewElectionTerm:
				d.State.StepDown(currentTerm)
			case ErrTooFewVotes:
				d.State.State(StateFollower)
			}
			return
		}
		// it wins the election
		d.State.LeaderID(d.State.ID())
		d.State.State(StateLeader)
	}()
	for {
		select {
		case <-d.stopChan:
			return
		case aer := <-d.State.AppendEntriesEvent():
			// https://raft.github.io/raft.pdf
			// While waiting for votes, a candidate may receive an
			// AppendEntries RPC from another server claiming to be
			// leader. If the leader’s term (included in its RPC) is at least
			// as large as the candidate’s current term, then the candidate
			// recognizes the leader as legitimate and returns to follower
			// state. If the term in the RPC is smaller than the candidate’s
			// current term, then the candidate rejects the RPC and continues
			// in candidate state.
			d.logger.Println("candidate got an AppendEntries from a leader", aer)
			if aer.Term >= d.State.Term() {
				d.State.StepDown(aer.Term)
				return
			}
		case newState := <-d.State.StateChanged():
			if newState == StateCandidate {
				continue
			}
			d.logger.Println("candidate state changed to", d.State.StateString(newState))
			return
		case e := <-d.State.ElectionTick():
			d.logger.Println("election timeout, restarting election", e)
			return
		}
	}
}

// leader is for when in StateLeader. The loop will continually send
// a heartbeat of AppendEntries to all peers at a rate of HeartbeatTimeoutMS.
func (d *Dinghy) leader() {
	d.logger.Println("entering leader state")
	err := d.OnLeader()
	if err != nil {
		d.logger.Errorln("executing OnLeader", err)
	}
	go d.AppendEntriesRequest()
	for {
		select {
		case <-d.stopChan:
			return
		case <-d.State.AppendEntriesEvent():
			// ignore any append entries to self.
			continue
		case newState := <-d.State.StateChanged():
			if newState == StateLeader {
				continue
			}
			d.logger.Println("leader state changed to", d.State.StateString(newState))
			return
		case h := <-d.State.HeartbeatTick():
			d.logger.Println("sending to peers AppendEntriesRequest", d.Nodes, h)
			currentTerm, err := d.AppendEntriesRequest()
			if err != nil {
				d.logger.Errorln("executing AppendEntriesRequest", err)
				switch err {
				case ErrNewElectionTerm:
					d.State.StepDown(currentTerm)
					return
				}
			}
		}
	}
}
