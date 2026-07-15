package monitor

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/frp-panel/frp-panel/internal/model"
)

func TestPollAllLimitsConcurrencyAndSkipsOverlap(t *testing.T) {
	db := openMonitorTestDB(t)
	for i := 0; i < 20; i++ {
		if err := db.Create(&model.Server{Name: "server", IP: "127.0.0.1", Status: "running"}).Error; err != nil {
			t.Fatalf("create server: %v", err)
		}
	}

	p := NewPoller(db, time.Second)
	var current atomic.Int32
	var maximum atomic.Int32
	release := make(chan struct{})
	started := make(chan struct{})
	var once sync.Once
	p.pollFn = func(*model.Server) {
		n := current.Add(1)
		for old := maximum.Load(); n > old && !maximum.CompareAndSwap(old, n); old = maximum.Load() {
		}
		once.Do(func() { close(started) })
		<-release
		current.Add(-1)
	}

	done := make(chan struct{})
	go func() {
		p.pollAll()
		close(done)
	}()
	<-started

	p.pollAll()
	close(release)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("poll did not finish")
	}

	if got := maximum.Load(); got > 8 {
		t.Fatalf("maximum concurrency = %d, want <= 8", got)
	}
}
