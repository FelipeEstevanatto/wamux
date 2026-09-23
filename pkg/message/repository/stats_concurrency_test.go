package message_repository

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

// A cold cache hit by many dashboard polls at once must run the whole-table
// aggregations once, not once per caller. sqlmock fails on any query that was
// not expected, so an un-serialised stampede would surface as errors here.
func TestGetStatsColdCacheRunsOnce(t *testing.T) {
	repo, mock := newMockRepo(t, WithAggregateCacheTTL(time.Minute))
	expectStatsQueries(mock, 42)

	const callers = 16
	var wg sync.WaitGroup
	errs := make(chan error, callers)

	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			stats, err := repo.GetStats()
			if err != nil {
				errs <- err
				return
			}
			if stats.Total != 42 {
				errs <- fmt.Errorf("total = %d, want 42", stats.Total)
			}
		}()
	}

	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("concurrent GetStats: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unexpected extra queries: %v", err)
	}
}
