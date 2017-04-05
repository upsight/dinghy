package dinghy

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"
)

// AppendEntriesRequest represents AppendEntries requests. Replication logging is ignored.
type AppendEntriesRequest struct {
	Term     int `json:"term"`
	LeaderID int `json:"leader_id"`
}

// appendEntriesResponse represents the response to an appendEntries. In
// dinghy this always returns success.
type appendEntriesResponse struct {
	Term    int    `json:"term"`
	Success bool   `json:"success"`
	Reason  string `json:"reason,omitempty"`
}

// requestVoteRequest represents a requestVote sent by a candidate after an
// election timeout.
type requestVoteRequest struct {
	Term        int `json:"term"`
	CandidateID int `json:"candidate_id"`
}

// requestVoteResponse represents the response to a requestVote.
type requestVoteResponse struct {
	Term        int    `json:"term"`
	VoteGranted bool   `json:"vote_granted"`
	Reason      string `json:"reason,omitempty"`
}

// RequestVoteRequest will broadcast a request for votes in order to update dinghy to
// either a follower or leader. If this candidate becomes leader error
// will return nil. The latest known term is always
// returned (this could be a newer term from another peer).
func (d *Dinghy) RequestVoteRequest() (int, error) {
	if d.State.State() == StateLeader {
		// no op
		d.logger.Println("already leader")
		return d.State.Term(), nil
	}

	rvr := requestVoteRequest{
		Term:        d.State.Term(),
		CandidateID: d.State.ID(),
	}
	method := "POST"
	route := d.routePrefix + RouteRequestVote
	body, err := json.Marshal(rvr)
	if err != nil {
		d.logger.Errorln("could not create payload", method, route, string(body))
		return d.State.Term(), err
	}
	responses := d.BroadcastRequest(d.Nodes, method, route, body, 0)
	votes := 0
LOOP:
	for i, resp := range responses {
		if resp == nil {
			// peer failed
			continue LOOP
		}
		defer resp.Body.Close()
		var rvr requestVoteResponse
		if err := json.NewDecoder(resp.Body).Decode(&rvr); err != nil {
			d.logger.Errorln("peer request returned invalid", d.Nodes[i], err)
			continue LOOP
		}
		if rvr.Term > d.State.Term() {
			// step down to a follower with the newer term
			d.logger.Printf("newer election term found %+v %s", rvr, d.State)
			return rvr.Term, ErrNewElectionTerm
		}
		if rvr.VoteGranted {
			d.logger.Printf("vote granted from %s %+v", d.Nodes[i], rvr)
			votes++
		}
	}

	if votes < (len(d.Nodes))/2 {
		d.logger.Printf("too few votes %d but need more than %d", votes, (len(d.Nodes))/2)
		return d.State.Term(), ErrTooFewVotes
	}

	d.logger.Printf("election won with %d votes, becoming leader %s", votes, d.State)
	return d.State.Term(), nil
}

// AppendEntriesRequest will broadcast an AppendEntries request to peers.
// In the raft protocol this deals with appending and processing the
// replication log, however for leader election this is unused.
// It returns the current term with any errors.
func (d *Dinghy) AppendEntriesRequest() (int, error) {
	aer := AppendEntriesRequest{
		Term:     d.State.Term(),
		LeaderID: d.State.ID(),
	}
	method := "POST"
	route := d.routePrefix + RouteAppendEntries
	body, err := json.Marshal(aer)
	if err != nil {
		d.logger.Errorln("could not create payload", method, route, string(body))
		return d.State.Term(), err
	}
	responses := d.BroadcastRequest(d.Nodes, method, route, body, d.State.heartbeatTimeoutMS/2)
	for i, resp := range responses {
		if resp == nil {
			// peer failed
			continue
		}
		defer resp.Body.Close()
		var aer appendEntriesResponse
		if err := json.NewDecoder(resp.Body).Decode(&aer); err != nil {
			d.logger.Errorln("peer request returned invalid", d.Nodes[i], err)
			continue
		}
		if aer.Term > d.State.Term() {
			d.logger.Printf("newer election term found %+v", aer)
			return aer.Term, ErrNewElectionTerm
		}
	}

	return d.State.Term(), nil
}

// BroadcastRequest will send a request to all other nodes in the system.
func (d *Dinghy) BroadcastRequest(peers []string, method, route string, body []byte, timeoutMS int) []*http.Response {
	responses := make([]*http.Response, len(peers))
	wg := &sync.WaitGroup{}
	for ind, peer := range peers {
		wg.Add(1)
		go func(i int, p string) {
			defer wg.Done()

			url := p
			if !strings.HasPrefix(url, "http") {
				url = "http://" + url + route
			}
			req, err := http.NewRequest(method, url, bytes.NewBuffer(body))
			if err != nil {
				d.logger.Errorln("could not create request", method, url, string(body))
				return
			}
			if timeoutMS > 0 {
				ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutMS)*time.Millisecond)
				defer cancel()
				req = req.WithContext(ctx)
			}
			resp, err := d.client.Do(req)
			if err != nil {
				d.logger.Errorln("failed request", err, method, url, string(body))
				return
			}
			responses[i] = resp
		}(ind, peer)
	}
	wg.Wait()
	return responses
}
