package ping

import (
	"context"
	"github.com/stretchr/testify/assert"
	"log"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestAtomic(t *testing.T) {
	c := uint32(0)
	gs := sync.WaitGroup{}
	for i := 0; i < runtime.NumCPU(); i++ {
		gs.Add(1)
		go func(cc *uint32) {
			n := 1024
			for n > 0 {
				atomic.AddUint32(cc, 1)
				n--
			}

			gs.Done()
		}(&c)
	}

	gs.Wait()
	t.Logf("get num: %v", c)
}

func TestPing(t *testing.T) {
	ctx, _ := context.WithTimeout(context.Background(), 4*time.Second)
	rttTimes, loss, err := Ping("119.23.221.167", ctx)
	if !assert.NoError(t, err) {
		t.Skipped()
	}


	var min, max, avg, jitter float64
	t.Logf("Ping Sum Number: %v, loss: %v", len(rttTimes)+loss, loss)
	if err := NetworkJitter(rttTimes, &min, &max, &avg, &jitter); err != nil {
		log.Fatal(err)
	}
	t.Logf("min %v max %v avgTime %v ms jitter %v ms", min, max, avg, jitter)
}

func TestNewWorkDelayTest(t *testing.T) {
	ctx, _ := context.WithTimeout(context.Background(), 4*time.Second)
	_, err := NewWorkDelayTest("stackoverflow.com", ctx)
	if !assert.NoError(t, err) {
		t.Skipped()
	}
}

