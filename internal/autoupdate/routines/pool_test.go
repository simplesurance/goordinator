package routines

import (
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/goleak"
)

func TestScheduleAndWait(t *testing.T) {
	var workDone [500]int32

	pool := NewPool(5)

	for i := range workDone {
		iPtr := &workDone[i]
		pool.Queue(func() {
			atomic.StoreInt32(iPtr, 1)
		})
	}

	pool.Wait()

	for i := range workDone {
		assert.Equal(t, int32(1), atomic.LoadInt32(&workDone[i]), "work %d not done", i)
	}
}

func TestQueuePanicsAfterWait(t *testing.T) {
	pool := NewPool(1)
	pool.Wait()

	assert.Panics(t, func() {
		pool.Queue(func() {})
	})
}

func TestWaitCanBeCalledMultipleTimes(t *testing.T) {
	pool := NewPool(10)
	pool.Wait()
	assert.NotPanics(t, pool.Wait)
}

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}
