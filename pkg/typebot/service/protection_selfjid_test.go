package typebot_service

import (
	"sync"
	"testing"
	"time"

	instance_model "github.com/evolution-foundation/evolution-go/pkg/instance/model"
	instance_repository "github.com/evolution-foundation/evolution-go/pkg/instance/repository"
)

type selfJidRepo struct {
	instance_repository.InstanceRepository
	mu    sync.Mutex
	calls int
	block chan struct{}
	jids  []*instance_model.Instance
}

func (r *selfJidRepo) GetAll(string) ([]*instance_model.Instance, error) {
	r.mu.Lock()
	r.calls++
	block := r.block
	r.mu.Unlock()
	if block != nil {
		<-block
	}
	return r.jids, nil
}

func (r *selfJidRepo) callCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls
}

func TestSelfJidCacheFirstLoadIsSynchronous(t *testing.T) {
	repo := &selfJidRepo{jids: []*instance_model.Instance{
		{Jid: "5588999999999:7@s.whatsapp.net"},
	}}
	c := newSelfJidCache(repo)

	if !c.contains("5588999999999@s.whatsapp.net") {
		t.Fatal("first load should resolve the self JID")
	}
	if c.contains("5514000000000@s.whatsapp.net") {
		t.Fatal("a foreign JID must not be treated as self")
	}
}

// After the first load, an expired cache is served immediately and refreshed in
// the background: contains runs per message and must not block on the database.
func TestSelfJidCacheServesStaleWhileRefreshing(t *testing.T) {
	repo := &selfJidRepo{jids: []*instance_model.Instance{
		{Jid: "5588999999999@s.whatsapp.net"},
	}}
	c := newSelfJidCache(repo)

	if !c.contains("5588999999999@s.whatsapp.net") {
		t.Fatal("warm-up load failed")
	}

	// Expire the cache and make the next refresh hang.
	c.mu.Lock()
	c.refreshed = time.Now().Add(-2 * c.ttl)
	c.mu.Unlock()

	repo.mu.Lock()
	repo.block = make(chan struct{})
	repo.mu.Unlock()

	done := make(chan bool, 1)
	go func() { done <- c.contains("5588999999999@s.whatsapp.net") }()

	select {
	case got := <-done:
		if !got {
			t.Fatal("stale cache should still answer with the last known value")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("contains blocked on the background refresh")
	}

	// Release the blocked refresh so the goroutine can finish.
	repo.mu.Lock()
	block := repo.block
	repo.mu.Unlock()
	close(block)
}

// Concurrent stale calls must collapse into a single refresh query.
func TestSelfJidCacheCollapsesConcurrentRefreshes(t *testing.T) {
	repo := &selfJidRepo{jids: []*instance_model.Instance{
		{Jid: "5588999999999@s.whatsapp.net"},
	}}
	c := newSelfJidCache(repo)

	c.contains("5588999999999@s.whatsapp.net") // first load -> 1 call

	c.mu.Lock()
	c.refreshed = time.Now().Add(-2 * c.ttl)
	c.mu.Unlock()

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c.contains("5588999999999@s.whatsapp.net")
		}()
	}
	wg.Wait()

	// Give the single background refresh a moment to run.
	deadline := time.Now().Add(time.Second)
	for repo.callCount() < 2 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}

	if got := repo.callCount(); got != 2 {
		t.Fatalf("expected exactly 2 GetAll calls (initial + one refresh), got %d", got)
	}
}
