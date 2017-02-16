# dinghy [![GoDoc](https://godoc.org/github.com/upsight/dinghy?status.svg)](http://godoc.org/github.com/upsight/dinghy) [![Build Status](https://travis-ci.org/upsight/dinghy.svg?branch=master)](https://travis-ci.org/upsight/dinghy)

Dinghy implements leader election using part of the raft protocol. It might
be useful if you have several workers but only want one of them at a time
doing things.

```
package main

import (
	"fmt"
	"github.com/upsight/dinghy"
)


func main() {
	addr := "localhost:8899"
	nodes := []string{"localhost:8899", "localhost:8898", "localhost:8897"}

	onLeader := func() error {
		fmt.Println("leader")
	}
	onFollower := func() error {
		fmt.Println("follower")
	}

	din, err := dinghy.New(
		addr,
		nodes,
		onLeader,
		onFollower,
		&dinghy.LogLogger{},
		dinghy.DefaultElectionTickRange,
		dinghy.DefaultHeartbeatTickRange,
	)
	if err != nil {
		log.Fatal(err)
	}
	for _, route := din.Routes() {
		http.Handle(route.Path, route.Handler)
	}
	log.Fatal(http.ListenAndServe(addr, nil))
}
```


![dinghy](dinghy.png)
