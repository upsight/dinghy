package dinghy

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDinghy_RequestVoteRequest(t *testing.T) {
	din := newDinghy(t)
	din.Nodes = []string{}

	tests := []struct {
		name         string
		startState   int
		endState     int
		useNodes     bool
		nodeResponse requestVoteResponse
		startTerm    int
		endTerm      int
		err          error
	}{
		{
			name:       "00 already leader",
			startState: StateLeader,
			endState:   StateLeader,
			useNodes:   false,
			nodeResponse: requestVoteResponse{
				Term:        din.State.Term(),
				VoteGranted: true,
			},
			startTerm: 1,
			endTerm:   1,
			err:       nil,
		},
		{
			name:       "01 no peers this state is leader",
			startState: StateLeader,
			endState:   StateLeader,
			useNodes:   false,
			nodeResponse: requestVoteResponse{
				Term:        din.State.Term(),
				VoteGranted: true,
			},
			startTerm: 1,
			endTerm:   1,
			err:       nil,
		},
		{
			name:       "02 vote granted",
			startState: StateCandidate,
			endState:   StateCandidate,
			useNodes:   true,
			nodeResponse: requestVoteResponse{
				Term:        din.State.Term(),
				VoteGranted: true,
			},
			startTerm: 1,
			endTerm:   1,
			err:       nil,
		},
		{
			name:       "03 deny vote",
			startState: StateFollower,
			endState:   StateFollower,
			useNodes:   true,
			nodeResponse: requestVoteResponse{
				Term:        din.State.Term(),
				VoteGranted: false,
			},
			startTerm: 1,
			endTerm:   1,
			err:       ErrTooFewVotes,
		},
		{
			name:       "04 step down",
			startState: StateFollower,
			endState:   StateFollower,
			useNodes:   true,
			nodeResponse: requestVoteResponse{
				Term:        din.State.Term() + 2,
				VoteGranted: true,
			},
			startTerm: 1,
			endTerm:   3,
			err:       ErrNewElectionTerm,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nodeResponse := tt.nodeResponse
			din.State.Term(tt.startTerm)
			din.State.State(tt.startState)
			if tt.useNodes {
				node0 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					json.NewEncoder(w).Encode(nodeResponse)
				}))
				node1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					json.NewEncoder(w).Encode(nodeResponse)
				}))
				node2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					json.NewEncoder(w).Encode(nodeResponse)
				}))
				node3 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					json.NewEncoder(w).Encode(nodeResponse)
				}))
				node4 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					json.NewEncoder(w).Encode(nodeResponse)
				}))
				din.Nodes = []string{node0.URL, node1.URL, node2.URL, node3.URL, node4.URL}
			} else {
				node0 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					json.NewEncoder(w).Encode(nodeResponse)
				}))
				din.Nodes = []string{node0.URL}
			}
			currentTerm, err := din.RequestVoteRequest()
			equals(t, tt.err, err)
			equals(t, din.State.StateString(tt.endState), din.State.StateString(din.State.State()))
			equals(t, tt.endTerm, currentTerm)
		})
	}
}

func TestDinghy_AppendEntriesRequest(t *testing.T) {
	din := newDinghy(t)

	tests := []struct {
		name         string
		startState   int
		endState     int
		useNodes     bool
		nodeResponse appendEntriesResponse
		startTerm    int
		endTerm      int
		err          error
	}{
		{
			name:       "00 follower",
			startState: StateFollower,
			endState:   StateFollower,
			useNodes:   true,
			nodeResponse: appendEntriesResponse{
				Term:    1,
				Success: true,
			},
			startTerm: 1,
			endTerm:   1,
			err:       nil,
		},
		{
			name:       "01 step down",
			startState: StateFollower,
			endState:   StateFollower,
			useNodes:   true,
			nodeResponse: appendEntriesResponse{
				Term:    2,
				Success: true,
			},
			startTerm: 1,
			endTerm:   2,
			err:       ErrNewElectionTerm,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nodeResponse := tt.nodeResponse
			din.State.Term(tt.startTerm)
			din.State.State(tt.startState)
			if tt.useNodes {
				node0 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					json.NewEncoder(w).Encode(nodeResponse)
				}))
				node1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					json.NewEncoder(w).Encode(nodeResponse)
				}))
				node2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					json.NewEncoder(w).Encode(nodeResponse)
				}))
				node3 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					json.NewEncoder(w).Encode(nodeResponse)
				}))
				node4 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					json.NewEncoder(w).Encode(nodeResponse)
				}))
				din.Nodes = []string{node0.URL, node1.URL, node2.URL, node3.URL, node4.URL}
			}
			currentTerm, err := din.AppendEntriesRequest()
			equals(t, tt.err, err)
			equals(t, din.State.StateString(tt.endState), din.State.StateString(din.State.State()))
			equals(t, tt.endTerm, currentTerm)
		})
	}
}
