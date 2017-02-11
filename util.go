package dinghy

import (
	"hash/fnv"
	"math/rand"
	"time"
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

// hashToInt will hash the input string and return
// it's int equivalent up to the max int given.
func hashToInt(s string, max int) int {
	h := fnv.New32a()
	h.Write([]byte(s))
	return int(h.Sum32() % uint32(max))
}

// randomTimeout returns a value that is between the min and 2x min.
func randomTimeout(min time.Duration) <-chan time.Time {
	if min == 0 {
		return nil
	}
	extra := (time.Duration(rand.Int63()) % min)
	return time.After(min + extra)
}
