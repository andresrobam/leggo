package lock

import (
	"slices"
	"sync"
)

var heldLocks = make([]string, 0)
var LockMutex sync.RWMutex

type LockReleaseMsg struct {
	Locks []string
}

func Lock(locks []string) {
	for _, lock := range locks {
		if !slices.Contains(heldLocks, lock) {
			heldLocks = append(heldLocks, lock)
		}
	}
}

func Unlock(locks []string) {
	heldLocks = slices.DeleteFunc(heldLocks, func(lock string) bool {
		return slices.Contains(locks, lock)
	})
}

func Overlap(locks []string) []string {
	overlap := make([]string, 0, len(heldLocks))
	for _, lock := range locks {
		if slices.Contains(heldLocks, lock) {
			overlap = append(overlap, lock)
		}
	}
	return overlap
}
