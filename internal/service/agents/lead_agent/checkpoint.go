package lead_agent

import (
	"fmt"
	"sync"
)

var checkpoint struct {
	mu  sync.Mutex
    mode string
	frozen bool 
	snapshot int
	snapshotFrozen int
}

func ResetCheckpointForTest() {
	checkpoint.mu.Lock()
	defer checkpoint.mu.Unlock()
	checkpoint.mode = ""
	checkpoint.frozen = false
	checkpoint.snapshot = 0
	checkpoint.snapshotFrozen = false
}

func FrozenCheckpointMode() (string , bool) {
	checkpoint.mu.Lock()
	defer checkpoint.mu.Unlock()
	return checkpoint.mode, checkpoint.frozen
}
