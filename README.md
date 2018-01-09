# dinghy [![GoDoc](https://godoc.org/github.com/upsight/dinghy?status.svg)](http://godoc.org/github.com/upsight/dinghy) [![Build Status](https://travis-ci.org/upsight/dinghy.svg?branch=master)](https://travis-ci.org/upsight/dinghy)

Dinghy implements leader election using part of the raft protocol. It might
be useful if you have several workers but only want one of them at a time
doing things.

```
package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/upsight/dinghy"
)

func main() {
	addr := flag.String("addr", "localhost:8899", "The address to listen on.")
	nodesList := flag.String("nodes", "localhost:8898,localhost:8897", "Comma separated list of host:port")
	flag.Parse()

	nodes := strings.Split(*nodesList, ",")
	nodes = append(nodes, *addr)

	onLeader := func() error {
		fmt.Println("leader")
		return nil
	}
	onFollower := func() error {
		fmt.Println("me follower")
		return nil
	}

	din, err := dinghy.New(
		*addr,
		nodes,
		onLeader,
		onFollower,
		&dinghy.LogLogger{Logger: log.New(os.Stderr, "logger: ", log.Lshortfile)},
		dinghy.DefaultElectionTickRange,
		dinghy.DefaultHeartbeatTickRange,
	)
	if err != nil {
		log.Fatal(err)
	}
	for _, route := range din.Routes() {
		http.HandleFunc(route.Path, route.Handler)
	}
	go func() {
		if err := din.Start(); err != nil {
			log.Fatal(err)
		}
	}()
	log.Fatal(http.ListenAndServe(*addr, nil))
}
```


![dinghy](dinghy.png)
