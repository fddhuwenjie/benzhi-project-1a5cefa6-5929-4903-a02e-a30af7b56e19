package store

import "sync"

const lockStripeCount = 64

type lockTable struct{ stripes [lockStripeCount]sync.Mutex }

func (t *lockTable) forCase(id string) *sync.Mutex {
	idx := 0
	for _, r := range id {
		idx = (idx*33 + int(r)) % lockStripeCount
	}
	return &t.stripes[idx]
}
