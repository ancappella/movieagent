package herness

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync/atomic"
	"time"
)

var idSeq atomic.Uint64

func newID(prefix string) string {
	var b [4]byte
	_, _ = rand.Read(b[:])
	n := idSeq.Add(1)
	return fmt.Sprintf("%s_%d_%s_%x", prefix, time.Now().UnixNano(), hex.EncodeToString(b[:]), n)
}
