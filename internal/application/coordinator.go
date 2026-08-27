package application

import "sync"

// Coordinator uses fixed stripes so arbitrary case IDs cannot grow lock state without bound.
type Coordinator struct{ stripes [64]sync.Mutex }

func (c *Coordinator) lock(caseID string) func() {
	idx := 0
	for _, r := range caseID {
		idx = (idx*31 + int(r)) % len(c.stripes)
	}
	m := &c.stripes[idx]
	m.Lock()
	return m.Unlock
}
