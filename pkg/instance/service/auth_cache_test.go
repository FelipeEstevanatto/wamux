package instance_service

import (
	"testing"
	"time"

	instance_model "github.com/evolution-foundation/evolution-go/pkg/instance/model"
	instance_repository "github.com/evolution-foundation/evolution-go/pkg/instance/repository"
	"github.com/patrickmn/go-cache"
)

// countingRepo embeds the repository interface and only implements the token
// lookup, counting how many times it is reached. The embedded nil interface
// satisfies the rest of the interface; those methods are never called here.
type countingRepo struct {
	instance_repository.InstanceRepository
	tokenCalls int
	instance   *instance_model.Instance
}

func (c *countingRepo) GetInstanceByToken(token string) (*instance_model.Instance, error) {
	c.tokenCalls++
	return c.instance, nil
}

func newCachedService(repo instance_repository.InstanceRepository) instances {
	return instances{
		instanceRepository: repo,
		authCache:          cache.New(authCacheTTL, 2*authCacheTTL),
	}
}

// The whole point of the cache: many authenticated requests must not each pay a
// DB round trip.
func TestGetInstanceByTokenServesRepeatedLookupsFromCache(t *testing.T) {
	repo := &countingRepo{instance: &instance_model.Instance{Id: "id-1", Token: "tok", Name: "name"}}
	svc := newCachedService(repo)

	for i := 0; i < 20; i++ {
		got, err := svc.GetInstanceByToken("tok")
		if err != nil {
			t.Fatalf("lookup %d: %v", i, err)
		}
		if got == nil || got.Id != "id-1" {
			t.Fatalf("lookup %d returned %+v", i, got)
		}
	}

	if repo.tokenCalls != 1 {
		t.Fatalf("expected 1 repository call, got %d", repo.tokenCalls)
	}
}

// The cached value is shared across requests, so callers must get a copy and
// must not be able to mutate the cached row.
func TestGetInstanceByTokenReturnsIndependentCopies(t *testing.T) {
	repo := &countingRepo{instance: &instance_model.Instance{Id: "id-1", Token: "tok", Name: "original"}}
	svc := newCachedService(repo)

	first, _ := svc.GetInstanceByToken("tok")
	first.Name = "mutated by caller"

	second, _ := svc.GetInstanceByToken("tok")
	if second.Name != "original" {
		t.Fatalf("cached row was mutated through a returned copy: %q", second.Name)
	}
}

// A write that changes the row must be visible on the next request, not after
// the TTL.
func TestInvalidateAuthCacheForcesRefetch(t *testing.T) {
	repo := &countingRepo{instance: &instance_model.Instance{Id: "id-1", Token: "tok", Name: "before"}}
	svc := newCachedService(repo)

	if _, err := svc.GetInstanceByToken("tok"); err != nil {
		t.Fatal(err)
	}
	if repo.tokenCalls != 1 {
		t.Fatalf("expected 1 call, got %d", repo.tokenCalls)
	}

	repo.instance = &instance_model.Instance{Id: "id-1", Token: "tok", Name: "after"}
	svc.invalidateAuthCache()

	got, err := svc.GetInstanceByToken("tok")
	if err != nil {
		t.Fatal(err)
	}
	if repo.tokenCalls != 2 {
		t.Fatalf("expected a refetch after invalidation, got %d calls", repo.tokenCalls)
	}
	if got.Name != "after" {
		t.Fatalf("expected fresh row, got %q", got.Name)
	}
}

// An empty token is never a valid identity and must not be cached.
func TestGetInstanceByTokenDoesNotCacheEmptyToken(t *testing.T) {
	repo := &countingRepo{instance: nil}
	svc := newCachedService(repo)

	for i := 0; i < 3; i++ {
		if _, err := svc.GetInstanceByToken(""); err != nil {
			t.Fatal(err)
		}
	}
	if repo.tokenCalls != 3 {
		t.Fatalf("empty token should not be cached, got %d calls", repo.tokenCalls)
	}
}

// A zero-value service (as used by some tests) must not panic.
func TestGetInstanceByTokenWithoutCache(t *testing.T) {
	repo := &countingRepo{instance: &instance_model.Instance{Id: "id-1", Token: "tok"}}
	svc := instances{instanceRepository: repo}

	if _, err := svc.GetInstanceByToken("tok"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.GetInstanceByToken("tok"); err != nil {
		t.Fatal(err)
	}
	if repo.tokenCalls != 2 {
		t.Fatalf("expected 2 calls with no cache, got %d", repo.tokenCalls)
	}
}

// fakeLatencyRepo models a Postgres round trip so the benchmark shows the
// per-request latency the cache removes.
type fakeLatencyRepo struct {
	instance_repository.InstanceRepository
	latency time.Duration
}

func (f *fakeLatencyRepo) GetInstanceByToken(token string) (*instance_model.Instance, error) {
	time.Sleep(f.latency)
	return &instance_model.Instance{Id: "id-1", Token: token, Name: "n"}, nil
}

func BenchmarkGetInstanceByTokenUncached(b *testing.B) {
	svc := instances{instanceRepository: &fakeLatencyRepo{latency: 200 * time.Microsecond}}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := svc.GetInstanceByToken("tok"); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkGetInstanceByTokenCached(b *testing.B) {
	svc := newCachedService(&fakeLatencyRepo{latency: 200 * time.Microsecond})
	// warm the cache
	if _, err := svc.GetInstanceByToken("tok"); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := svc.GetInstanceByToken("tok"); err != nil {
			b.Fatal(err)
		}
	}
}
