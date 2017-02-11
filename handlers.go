package dinghy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
)

const (
	// RouteID for id requests.
	RouteID = "/id"
	// RouteRequestVote for request vote requests.
	RouteRequestVote = "/requestvote"
	// RouteAppendEntries for append entries requests.
	RouteAppendEntries = "/appendentries"
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
	}
	return routes
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

		// The request is from an old term so reject
		if ae.Term < d.State.Term() {
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
		}

		// Step down and reset state on newer term.
		if ae.Term > d.State.Term() {
			d.logger.Printf("got AppendEntries request from newer term, stepping down %+v %s", ae, d.State)
			d.State.StepDown(ae.Term)
		}

		// ok
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
