package dinghy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
)

const (
	// RouteAppendEntries for append entries requests.
	RouteAppendEntries = "/appendentries"
	// RouteID for id requests.
	RouteID = "/id"
	// RouteRequestVote for request vote requests.
	RouteRequestVote = "/requestvote"
	// RouteStatus will render the current nodes full state.
	RouteStatus = "/status"
	// RouteStepDown to force a node to step down.
	RouteStepDown = "/stepdown"
)

var (
	emptyAppendEntriesResponse bytes.Buffer
	emptyRequestVoteResponse   bytes.Buffer
)

func init() {
	json.NewEncoder(&emptyAppendEntriesResponse).Encode(appendEntriesResponse{})
	json.NewEncoder(&emptyRequestVoteResponse).Encode(requestVoteResponse{})
}

// Route holds path and handler information.
type Route struct {
	Path    string
	Handler func(http.ResponseWriter, *http.Request)
}

// Routes create the routes required for leader election.
func (d *Dinghy) Routes() []*Route {
	routes := []*Route{
		&Route{d.routePrefix + RouteID, d.IDHandler()},
		&Route{d.routePrefix + RouteRequestVote, d.RequestVoteHandler()},
		&Route{d.routePrefix + RouteAppendEntries, d.AppendEntriesHandler()},
		&Route{d.routePrefix + RouteStatus, d.StatusHandler()},
		&Route{d.routePrefix + RouteStepDown, d.StepDownHandler()},
	}
	return routes
}

// StepDownHandler (POST) will force the node to step down to a follower state.
func (d *Dinghy) StepDownHandler() func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "PUT" {
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
			return
		}
		d.State.State(StateFollower)
		fmt.Fprintln(w, d.State)
	}
}

// StatusHandler (GET) returns the nodes full state.
func (d *Dinghy) StatusHandler() func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
			return
		}
		fmt.Fprintln(w, d.State)
	}
}

// IDHandler (GET) returns the nodes id.
func (d *Dinghy) IDHandler() func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
			return
		}
		fmt.Fprintln(w, d.State.ID())
	}
}

// RequestVoteHandler (POST) handles requests for votes
func (d *Dinghy) RequestVoteHandler() func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Body != nil {
			defer r.Body.Close()
		}
		if r.Method != "POST" {
			d.logger.Errorf("invalid method %s", r.Method)
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
			return
		}

		var rv requestVoteRequest
		if err := json.NewDecoder(r.Body).Decode(&rv); err != nil {
			d.logger.Errorf("%s", err)
			http.Error(w, emptyRequestVoteResponse.String(), http.StatusBadRequest)
			return
		}

		switch {
		case rv.Term < d.State.Term():
			// The request is from an old term so reject
			d.logger.Printf("got RequestVote request from older term, rejecting %+v %s", rv, d.State)
			rvResp := requestVoteResponse{
				Term:        d.State.Term(),
				VoteGranted: false,
				Reason:      fmt.Sprintf("term %d < %d", rv.Term, d.State.Term()),
			}
			if d.State.State() == StateLeader {
				rvResp.Reason = "already leader"
			}
			if err := json.NewEncoder(w).Encode(rvResp); err != nil {
				http.Error(w, emptyRequestVoteResponse.String(), http.StatusInternalServerError)
			}
			return
		case (d.State.VotedFor() != NoVote) && (d.State.VotedFor() != rv.CandidateID):
			// don't double vote if already voted for a term
			d.logger.Printf("got RequestVote request but already voted, rejecting %+v %s", rv, d.State)
			rvResp := requestVoteResponse{
				Term:        d.State.Term(),
				VoteGranted: false,
				Reason:      fmt.Sprintf("already cast vote for %d", d.State.VotedFor()),
			}
			if err := json.NewEncoder(w).Encode(rvResp); err != nil {
				http.Error(w, emptyRequestVoteResponse.String(), http.StatusInternalServerError)
			}
			return
		case rv.Term > d.State.Term():
			// Step down and reset state on newer term.
			d.logger.Printf("got RequestVote request from newer term, stepping down %+v %s", rv, d.State)
			d.State.StepDown(rv.Term)
			d.State.LeaderID(UnknownLeaderID)
		}

		// ok and vote for candidate
		// reset election timeout
		d.logger.Printf("got RequestVote request, voting for candidate id %d %s", rv.CandidateID, d.State)
		d.State.VotedFor(rv.CandidateID)
		//defer d.State.HeartbeatReset(true)
		rvResp := requestVoteResponse{
			Term:        d.State.Term(),
			VoteGranted: true,
		}
		if err := json.NewEncoder(w).Encode(rvResp); err != nil {
			http.Error(w, emptyRequestVoteResponse.String(), http.StatusInternalServerError)
		}
		return
	}
}

// AppendEntriesHandler (POST) handles append entry requests
func (d *Dinghy) AppendEntriesHandler() func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Body != nil {
			defer r.Body.Close()
		}
		if r.Method != "POST" {
			d.logger.Errorf("invalid method %s", r.Method)
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
			return
		}

		var ae AppendEntriesRequest
		if err := json.NewDecoder(r.Body).Decode(&ae); err != nil {
			d.logger.Errorf("%s", err)
			http.Error(w, emptyRequestVoteResponse.String(), http.StatusBadRequest)
			return
		}

		// There are a few cases here:
		// 1. The request is from an old term so reject
		// 2. The request is from a newer term so step down
		// 3. If we are a follower and get an append entries continue on with success
		// 4. If we are not a follower and get an append entries, it means there is
		//    another leader already and we should step down.
		switch {
		case ae.Term < d.State.Term():
			d.logger.Printf("got AppendEntries request from older term, rejecting %+v %s", ae, d.State)
			aeResp := appendEntriesResponse{
				Term:    d.State.Term(),
				Success: false,
				Reason:  fmt.Sprintf("term %d < %d", ae.Term, d.State.Term()),
			}
			if err := json.NewEncoder(w).Encode(aeResp); err != nil {
				http.Error(w, emptyRequestVoteResponse.String(), http.StatusInternalServerError)
			}
			return
		case ae.Term > d.State.Term():
			d.logger.Printf("got AppendEntries request from newer term, stepping down %+v %s", ae, d.State)
			d.State.StepDown(ae.Term)
		case ae.Term == d.State.Term():
			// ignore request to self and only step down if not in follower state.
			if ae.LeaderID != d.State.ID() && d.State.State() != StateFollower {
				d.logger.Printf("got AppendEntries request from another leader with the same term, stepping down %+v %s", ae, d.State)
				d.State.StepDown(ae.Term)
			}
		}

		// ok
		d.State.LeaderID(ae.LeaderID)
		d.State.AppendEntriesEvent(&ae)
		aeResp := appendEntriesResponse{
			Term:    d.State.Term(),
			Success: true,
		}
		if err := json.NewEncoder(w).Encode(aeResp); err != nil {
			http.Error(w, emptyRequestVoteResponse.String(), http.StatusInternalServerError)
		}
		return
	}
}
