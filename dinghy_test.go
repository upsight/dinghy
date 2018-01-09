package dinghy

import (
	"fmt"
	"io/ioutil"
	"log"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
	"time"
)

// assert fails the test if the condition is false.
func assert(tb testing.TB, condition bool, msg string, v ...interface{}) {
	if !condition {
		_, file, line, _ := runtime.Caller(1)
		fmt.Printf("\033[31m%s:%d: "+msg+"\033[39m\n\n", append([]interface{}{filepath.Base(file), line}, v...)...)
		tb.FailNow()
	}
}

// ok fails the test if an err is not nil.
func ok(tb testing.TB, err error) {
	if err != nil {
		_, file, line, _ := runtime.Caller(1)
		fmt.Printf("\033[31m%s:%d: unexpected error: %s\033[39m\n\n", filepath.Base(file), line, err.Error())
		tb.FailNow()
	}
}

// equals fails the test if exp is not equal to act.
func equals(tb testing.TB, exp, act interface{}) {
	if !reflect.DeepEqual(exp, act) {
		_, file, line, _ := runtime.Caller(1)
		fmt.Printf("\033[31m%s:%d:\n\n\texp: %#v\n\n\tgot: %#v\033[39m\n\n", filepath.Base(file), line, exp, act)
		tb.FailNow()
	}
}

func newDinghy(t testing.TB) *Dinghy {
	l := log.New(ioutil.Discard, "", log.Ldate|log.Ltime|log.Lshortfile)
	ll := &LogLogger{l}
	din, err := New(
		"localhost:9999",
		[]string{"localhost:9999", "localhost:9998"},
		DefaultOnLeader,
		DefaultOnFollower,
		ll,
		10,
		10,
	)
	ok(t, err)
	return din
}

func TestNew(t *testing.T) {
	type args struct {
		addr       string
		nodes      []string
		onLeader   func() error
		onFollower func() error
	}
	tests := []struct {
		name            string
		args            args
		want            *Dinghy
		wantErr         bool
		wantLeaderErr   bool
		wantFollowerErr bool
	}{
		{
			"00 init defaults",
			args{
				addr:  "localhost:9999",
				nodes: []string{"localhost:9999", "localhost:9998"},
			},
			&Dinghy{
				Addr:       "localhost:9999",
				Nodes:      []string{"localhost:9999", "localhost:9998"},
				OnLeader:   DefaultOnLeader,
				OnFollower: DefaultOnFollower,
			},
			false,
			false,
			false,
		},
		{
			"01 event methods",
			args{
				addr:  "localhost:9999",
				nodes: []string{"localhost:9999", "localhost:9998"},
			},
			&Dinghy{
				Addr:       "localhost:9999",
				Nodes:      []string{"localhost:9999", "localhost:9998"},
				OnLeader:   func() error { return fmt.Errorf("") },
				OnFollower: func() error { return fmt.Errorf("") },
			},
			false,
			false,
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := log.New(ioutil.Discard, "", 0)
			ll := &LogLogger{l}
			got, err := New(tt.args.addr, tt.args.nodes, tt.args.onLeader, tt.args.onFollower, ll, 10, 10)
			if (err != nil) != tt.wantErr {
				t.Errorf("New() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			equals(t, got.Addr, tt.want.Addr)
			equals(t, got.Nodes, tt.want.Nodes)
			got.mu.Lock()
			defer got.mu.Unlock()
			err = got.OnLeader()
			if (err != nil) != tt.wantLeaderErr {
				t.Errorf("OnLeader() error = %v, wantErr %v", err, tt.wantLeaderErr)
				return
			}
			err = got.OnFollower()
			if (err != nil) != tt.wantFollowerErr {
				t.Errorf("OnFollower() error = %v, wantErr %v", err, tt.wantFollowerErr)
				return
			}
		})
	}
}

func TestDinghy_StartStop(t *testing.T) {
	din := newDinghy(t)
	go func() {
		err := din.Start()
		if err != nil {
			t.Fatal(err)
		}
	}()
	time.Sleep(time.Millisecond * 10)
	din.Stop()

	// Start with an invalid state
	din.State.State(99)
	err := din.Start()
	equals(t, "unknown state 99", err.Error())
}

func TestDinghy_follower(t *testing.T) {
	// coverage only
	din := newDinghy(t)
	go din.Start()
	go din.follower()
	time.Sleep(time.Millisecond * 10)
	din.Stop()

	// state change
	go din.Start()
	go din.follower()
	din.State.State(StateCandidate)
	time.Sleep(time.Millisecond * 20)
	din.Stop()

	// append entries
	din.State.State(StateFollower)
	go din.Start()
	go din.follower()
	din.State.AppendEntriesEvent(&AppendEntriesRequest{5, din.State.ID()})
	time.Sleep(time.Millisecond * 20)
	din.Stop()

	// heartbeat timeout
	din.State.State(StateFollower)
	go din.Start()
	go din.leader()
	time.Sleep(time.Millisecond * 10)
	din.Stop()

	wantErr := fmt.Errorf("test")
	din.mu.Lock()
	din.OnFollower = func() error { return wantErr }
	din.mu.Unlock()
	err := din.Start()
	equals(t, wantErr, err)
}

func TestDinghy_candidate(t *testing.T) {
	// coverage only
	din := newDinghy(t)
	go din.Start()
	go din.candidate()
	time.Sleep(time.Millisecond * 10)
	din.Stop()

	// state change
	go din.Start()
	go din.leader()
	din.State.State(StateFollower)
	time.Sleep(time.Millisecond * 20)
	din.Stop()

	// election timeout
	din.State.State(StateCandidate)
	go din.Start()
	go din.leader()
	time.Sleep(time.Millisecond * 10)
	din.Stop()
}

func TestDinghy_leader(t *testing.T) {
	// coverage only
	din := newDinghy(t)
	go din.Start()
	go din.leader()
	time.Sleep(time.Millisecond * 10)
	din.Stop()

	// state change
	go din.Start()
	go din.leader()
	din.State.State(StateFollower)
	time.Sleep(time.Millisecond * 20)
	din.Stop()

	// heartbeat tick
	din.State.State(StateLeader)
	go din.Start()
	go din.leader()
	time.Sleep(time.Millisecond * 10)
	din.Stop()

	wantErr := fmt.Errorf("test")
	din.mu.Lock()
	din.OnLeader = func() error { return wantErr }
	din.mu.Unlock()
	err := din.leader()
	equals(t, wantErr, err)
	time.Sleep(time.Millisecond * 10)
	equals(t, StateFollower, din.State.State())
}
