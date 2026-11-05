package killswitch

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// The bug this type replaces: a signal reached one of two receivers.
func TestFireReachesEveryHolder(t *testing.T) {
	s := New()
	var woken atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-s.Done()
			woken.Add(1)
		}()
	}
	s.Fire(Stop)
	wg.Wait()
	if woken.Load() != 5 {
		t.Fatalf("woken = %d, want 5", woken.Load())
	}
}

// A holder that looks after the fire must still see it, however late.
func TestLateObserverStillSeesFire(t *testing.T) {
	s := New()
	s.Fire(Restart)
	select {
	case <-s.Done():
	case <-time.After(time.Second):
		t.Fatal("late observer missed the signal")
	}
	if !s.Fired() || s.Mode() != Restart {
		t.Fatalf("Fired=%v Mode=%v", s.Fired(), s.Mode())
	}
}

func TestFirstFireWins(t *testing.T) {
	s := New()
	if !s.Fire(Detach) {
		t.Fatal("first Fire should report true")
	}
	if s.Fire(Restart) {
		t.Fatal("second Fire should report false")
	}
	if s.Mode() != Detach {
		t.Fatalf("Mode = %v, want Detach", s.Mode())
	}
}

func TestConcurrentFireIsSafe(t *testing.T) {
	s := New()
	var wins atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if s.Fire(Stop) {
				wins.Add(1)
			}
		}()
	}
	wg.Wait()
	if wins.Load() != 1 {
		t.Fatalf("wins = %d, want 1", wins.Load())
	}
}

func TestNilSwitchNeverFires(t *testing.T) {
	var s *Switch
	if s.Fire(Stop) || s.Fired() {
		t.Fatal("nil switch must not fire")
	}
	select {
	case <-s.Done():
		t.Fatal("nil Done must block")
	default:
	}
}
