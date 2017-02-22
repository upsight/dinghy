package dinghy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
)

func TestDinghy_Handlers(t *testing.T) {
	din := newDinghy(t)
	r := din.Routes()
	equals(t, 5, len(r))
}

func TestDinghy_StatusHandler(t *testing.T) {
	din := newDinghy(t)
	handler := din.StatusHandler()

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", din.routePrefix+RouteStatus, nil)
	handler(w, req)
	equals(t, http.StatusMethodNotAllowed, w.Code)

	w = httptest.NewRecorder()
	req = httptest.NewRequest("GET", din.routePrefix+RouteStatus, nil)
	handler(w, req)

	equals(t, http.StatusOK, w.Code)
	want := Status{
		ID:       1613,
		LeaderID: 0,
		State:    "follower",
		Term:     1,
		VotedFor: 0,
	}
	var got Status
	json.Unmarshal(w.Body.Bytes(), &got)
	equals(t, want, got)
}

func TestDinghy_StepDownHandler(t *testing.T) {
	din := newDinghy(t)
	din.State.State(StateLeader)
	handler := din.StepDownHandler()

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", din.routePrefix+RouteStepDown, nil)
	handler(w, req)
	equals(t, http.StatusMethodNotAllowed, w.Code)

	w = httptest.NewRecorder()
	req = httptest.NewRequest("PUT", din.routePrefix+RouteStepDown, nil)
	handler(w, req)

	equals(t, http.StatusOK, w.Code)
	want := Status{
		ID:       1613,
		LeaderID: 0,
		State:    "follower",
		Term:     1,
		VotedFor: 0,
	}
	var got Status
	json.Unmarshal(w.Body.Bytes(), &got)
	equals(t, want, got)
}

func TestDinghy_IDHandler(t *testing.T) {
	din := newDinghy(t)
	idHandler := din.IDHandler()

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", din.routePrefix+RouteID, nil)
	idHandler(w, req)
	equals(t, http.StatusMethodNotAllowed, w.Code)

	w = httptest.NewRecorder()
	req = httptest.NewRequest("GET", din.routePrefix+RouteID, nil)
	idHandler(w, req)
	equals(t, http.StatusOK, w.Code)
	equals(t, strconv.Itoa(din.State.ID())+"\n", w.Body.String())
}

func TestDinghy_RequestVoteHandler(t *testing.T) {
	din := newDinghy(t)
	rvHandler := din.RequestVoteHandler()

	tests := []struct {
		name          string
		method        string
		req           *requestVoteRequest
		resp          *requestVoteResponse
		statusCode    int
		startState    int
		endState      int
		startTerm     int
		endTerm       int
		startVotedFor int
		endVotedFor   int
		startLeaderID int
		endLeaderID   int
	}{
		{
			name:          "00 method not allowed",
			method:        "GET",
			req:           nil,
			resp:          nil,
			statusCode:    http.StatusMethodNotAllowed,
			startState:    StateFollower,
			endState:      StateFollower,
			startTerm:     1,
			endTerm:       1,
			startVotedFor: NoVote,
			endVotedFor:   NoVote,
			startLeaderID: UnknownLeaderID,
			endLeaderID:   UnknownLeaderID,
		},
		{
			name:   "01 ok",
			method: "POST",
			req: &requestVoteRequest{
				Term:        din.State.Term(),
				CandidateID: din.State.ID(),
			},
			resp: &requestVoteResponse{
				Term:        din.State.Term(),
				VoteGranted: true,
			},
			statusCode:    http.StatusOK,
			startState:    StateFollower,
			endState:      StateFollower,
			startTerm:     1,
			endTerm:       1,
			startVotedFor: NoVote,
			endVotedFor:   din.State.ID(),
			startLeaderID: UnknownLeaderID,
			endLeaderID:   UnknownLeaderID,
		},
		{
			name:   "02 request from an old term so rejected",
			method: "POST",
			req: &requestVoteRequest{
				Term:        din.State.Term() - 1,
				CandidateID: din.State.ID(),
			},
			resp: &requestVoteResponse{
				Term:        din.State.Term(),
				VoteGranted: false,
				Reason:      "term 0 < 1",
			},
			statusCode:    http.StatusOK,
			startState:    StateFollower,
			endState:      StateFollower,
			startTerm:     1,
			endTerm:       1,
			startVotedFor: NoVote,
			endVotedFor:   NoVote,
			startLeaderID: UnknownLeaderID,
			endLeaderID:   UnknownLeaderID,
		},
		{
			name:   "03 request from an old term so rejected already leader",
			method: "POST",
			req: &requestVoteRequest{
				Term:        din.State.Term() - 1,
				CandidateID: din.State.ID(),
			},
			resp: &requestVoteResponse{
				Term:        din.State.Term(),
				VoteGranted: false,
				Reason:      "already leader",
			},
			statusCode:    http.StatusOK,
			startState:    StateLeader,
			endState:      StateLeader,
			startTerm:     1,
			endTerm:       1,
			startVotedFor: NoVote,
			endVotedFor:   NoVote,
			startLeaderID: din.State.ID(),
			endLeaderID:   din.State.ID(),
		},
		{
			name:   "04 double vote",
			method: "POST",
			req: &requestVoteRequest{
				Term:        din.State.Term(),
				CandidateID: din.State.ID() + 1,
			},
			resp: &requestVoteResponse{
				Term:        din.State.Term(),
				VoteGranted: false,
				Reason:      fmt.Sprintf("already cast vote for %d", din.State.ID()),
			},
			statusCode:    http.StatusOK,
			startState:    StateFollower,
			endState:      StateFollower,
			startTerm:     1,
			endTerm:       1,
			startVotedFor: din.State.ID(),
			endVotedFor:   din.State.ID(),
			startLeaderID: din.State.ID(),
			endLeaderID:   din.State.ID(),
		},
		{
			name:   "04 newer term",
			method: "POST",
			req: &requestVoteRequest{
				Term:        din.State.Term() + 1,
				CandidateID: din.State.ID(),
			},
			resp: &requestVoteResponse{
				Term:        2,
				VoteGranted: true,
			},
			statusCode:    http.StatusOK,
			startState:    StateFollower,
			endState:      StateFollower,
			startTerm:     1,
			endTerm:       2,
			startVotedFor: din.State.ID(),
			endVotedFor:   din.State.ID(),
			startLeaderID: UnknownLeaderID,
			endLeaderID:   UnknownLeaderID,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			din.State.State(tt.startState)
			din.State.Term(tt.startTerm)
			din.State.VotedFor(tt.startVotedFor)
			din.State.LeaderID(tt.startLeaderID)
			w := httptest.NewRecorder()
			var req *http.Request
			var body *bytes.Buffer
			if tt.req != nil {
				b, _ := json.Marshal(tt.req)
				body = bytes.NewBuffer(b)
				req = httptest.NewRequest(tt.method, din.routePrefix+RouteRequestVote, body)
			} else {
				req = httptest.NewRequest(tt.method, din.routePrefix+RouteRequestVote, nil)
			}
			rvHandler(w, req)
			equals(t, tt.statusCode, w.Code)
			if tt.resp != nil {
				want, _ := json.Marshal(tt.resp)
				equals(t, string(want)+"\n", w.Body.String())
			}
			equals(t, din.State.StateString(tt.endState), din.State.StateString(din.State.State()))
			equals(t, tt.endTerm, din.State.Term())
			equals(t, tt.endVotedFor, din.State.VotedFor())
			equals(t, tt.endLeaderID, din.State.LeaderID())
		})
	}
}

func TestDinghy_AppendEntriesHandler(t *testing.T) {
	din := newDinghy(t)
	aeHandler := din.AppendEntriesHandler()

	tests := []struct {
		name          string
		method        string
		req           *AppendEntriesRequest
		wantResp      *appendEntriesResponse
		statusCode    int
		startState    int
		endState      int
		startTerm     int
		endTerm       int
		startVotedFor int
		endVotedFor   int
		startLeaderID int
		endLeaderID   int
	}{
		{
			name:          "00 method not allowed",
			method:        "GET",
			req:           nil,
			wantResp:      nil,
			statusCode:    http.StatusMethodNotAllowed,
			startState:    StateFollower,
			endState:      StateFollower,
			startTerm:     1,
			endTerm:       1,
			startVotedFor: NoVote,
			endVotedFor:   NoVote,
			startLeaderID: UnknownLeaderID,
			endLeaderID:   UnknownLeaderID,
		},
		{
			name:   "01 request from an old term so rejected",
			method: "POST",
			req: &AppendEntriesRequest{
				Term:     din.State.Term() - 1,
				LeaderID: din.State.ID(),
			},
			wantResp: &appendEntriesResponse{
				Term:    din.State.Term(),
				Success: false,
				Reason:  "term 0 < 1",
			},
			statusCode:    http.StatusOK,
			startState:    StateFollower,
			endState:      StateFollower,
			startTerm:     1,
			endTerm:       1,
			startVotedFor: NoVote,
			endVotedFor:   NoVote,
			startLeaderID: UnknownLeaderID,
			endLeaderID:   UnknownLeaderID,
		},
		{
			name:   "02 request from a newer term so step down",
			method: "POST",
			req: &AppendEntriesRequest{
				Term:     din.State.Term() + 1,
				LeaderID: 999,
			},
			wantResp: &appendEntriesResponse{
				Term:    2,
				Success: true,
			},
			statusCode:    http.StatusOK,
			startState:    StateCandidate,
			endState:      StateFollower,
			startTerm:     1,
			endTerm:       2,
			startVotedFor: NoVote,
			endVotedFor:   NoVote,
			startLeaderID: UnknownLeaderID,
			endLeaderID:   999,
		},
		{
			name:   "03 request from a equal term so step down",
			method: "POST",
			req: &AppendEntriesRequest{
				Term:     2,
				LeaderID: 999,
			},
			wantResp: &appendEntriesResponse{
				Term:    2,
				Success: true,
			},
			statusCode:    http.StatusOK,
			startState:    StateLeader,
			endState:      StateFollower,
			startTerm:     2,
			endTerm:       2,
			startVotedFor: NoVote,
			endVotedFor:   NoVote,
			startLeaderID: UnknownLeaderID,
			endLeaderID:   999,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			din.State.State(tt.startState)
			din.State.Term(tt.startTerm)
			din.State.VotedFor(tt.startVotedFor)
			din.State.LeaderID(tt.startLeaderID)
			w := httptest.NewRecorder()
			var req *http.Request
			var body *bytes.Buffer
			if tt.req != nil {
				b, _ := json.Marshal(tt.req)
				body = bytes.NewBuffer(b)
				req = httptest.NewRequest(tt.method, din.routePrefix+RouteAppendEntries, body)
			} else {
				req = httptest.NewRequest(tt.method, din.routePrefix+RouteAppendEntries, nil)
			}
			aeHandler(w, req)
			equals(t, tt.statusCode, w.Code)
			if tt.wantResp != nil {
				want, _ := json.Marshal(tt.wantResp)
				equals(t, string(want)+"\n", w.Body.String())
			}
			equals(t, din.State.StateString(tt.endState), din.State.StateString(din.State.State()))
			equals(t, tt.endTerm, din.State.Term())
			equals(t, tt.endVotedFor, din.State.VotedFor())
			equals(t, tt.endLeaderID, din.State.LeaderID())
		})
	}
}
