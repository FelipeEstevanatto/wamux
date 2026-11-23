package cloud

import (
	"context"
	"errors"
	"math"
	"sync"
	"time"

	cloud_model "github.com/felipeestevanatto/wamux/pkg/cloud/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// PhoneLimiter paces Cloud sends per business phone number. It is intentionally
// small so a single process can use it in tests while a deployment can share
// the Postgres-backed implementation across replicas.
type PhoneLimiter interface {
	Wait(ctx context.Context, phoneNumberID string) error
	Observe(phoneNumberID string, status int, retryAfter time.Duration)
	EffectiveRPS(phoneNumberID string) float64
}

// NewPhoneLimiter returns a Postgres-backed limiter when available and an
// in-memory limiter otherwise. SQLite is common in local installs and tests;
// keeping the fallback process-local avoids requiring a new service.
func NewPhoneLimiter(db *gorm.DB, baseRPS float64) PhoneLimiter {
	if baseRPS <= 0 {
		baseRPS = 8
	}
	if db != nil && db.Dialector != nil && db.Dialector.Name() == "postgres" {
		return &dbPhoneLimiter{db: db, baseRPS: baseRPS}
	}
	return NewMemoryPhoneLimiter(baseRPS)
}

// MemoryPhoneLimiter keeps one adaptive bucket per phone number in this
// process. It is the fallback for SQLite and for tools that do not open a DB.
type MemoryPhoneLimiter struct {
	mu          sync.Mutex
	base        float64
	phones      map[string]*adaptiveLimiter
	pausedUntil map[string]time.Time
}

func NewMemoryPhoneLimiter(baseRPS float64) *MemoryPhoneLimiter {
	if baseRPS <= 0 {
		baseRPS = 8
	}
	return &MemoryPhoneLimiter{base: baseRPS, phones: map[string]*adaptiveLimiter{}, pausedUntil: map[string]time.Time{}}
}

func (l *MemoryPhoneLimiter) limiter(phone string) *adaptiveLimiter {
	l.mu.Lock()
	defer l.mu.Unlock()
	lim := l.phones[phone]
	if lim == nil {
		lim = newAdaptiveLimiter(l.base)
		l.phones[phone] = lim
	}
	return lim
}

func (l *MemoryPhoneLimiter) Wait(ctx context.Context, phone string) error {
	l.mu.Lock()
	pause := time.Until(l.pausedUntil[phone])
	l.mu.Unlock()
	if pause > 0 {
		timer := time.NewTimer(pause)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	return l.limiter(phone).Wait(ctx)
}

func (l *MemoryPhoneLimiter) Observe(phone string, status int, retryAfter time.Duration) {
	lim := l.limiter(phone)
	lim.mu.Lock()
	defer lim.mu.Unlock()
	if status == 429 {
		lim.penalizeLocked()
		if retryAfter > 0 {
			l.mu.Lock()
			l.pausedUntil[phone] = time.Now().Add(retryAfter)
			l.mu.Unlock()
		}
		return
	}
	lim.recoverLocked()
}

func (l *MemoryPhoneLimiter) EffectiveRPS(phone string) float64 {
	return l.limiter(phone).RPS()
}

// dbPhoneLimiter stores a token bucket in cloud_rate_buckets. A row lock makes
// concurrent workers and replicas share the same budget. It never holds a DB
// transaction while sleeping, so a slow phone cannot block the pool.
type dbPhoneLimiter struct {
	db      *gorm.DB
	baseRPS float64
}

func (l *dbPhoneLimiter) Wait(ctx context.Context, phone string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	for {
		var wait time.Duration
		now := time.Now()
		err := l.db.Transaction(func(tx *gorm.DB) error {
			var b cloud_model.RateBucket
			err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
				Where("phone_number_id = ?", phone).First(&b).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				b = cloud_model.RateBucket{
					PhoneNumberID: phone, Tokens: l.baseRPS,
					EffectiveRPS: l.baseRPS, UpdatedAt: now,
				}
				if err := tx.Create(&b).Error; err != nil {
					return err
				}
			} else if err != nil {
				return err
			}
			rate := b.EffectiveRPS
			if rate <= 0 {
				rate = l.baseRPS
			}
			if b.PausedUntil != nil && now.Before(*b.PausedUntil) {
				wait = time.Until(*b.PausedUntil) + time.Millisecond
				b.UpdatedAt = now
				return tx.Save(&b).Error
			}
			if b.Last429At != nil && now.Sub(*b.Last429At) > 10*time.Second && rate < l.baseRPS {
				rate = math.Min(l.baseRPS, rate*1.25)
			}
			elapsed := now.Sub(b.UpdatedAt).Seconds()
			if elapsed < 0 {
				elapsed = 0
			}
			tokens := math.Min(l.baseRPS, b.Tokens+elapsed*rate)
			b.EffectiveRPS = rate
			b.UpdatedAt = now
			if tokens >= 1 {
				b.Tokens = tokens - 1
				return tx.Save(&b).Error
			}
			b.Tokens = tokens
			wait = time.Duration((1-tokens)/rate*float64(time.Second)) + time.Millisecond
			return tx.Save(&b).Error
		})
		if err != nil {
			return err
		}
		if wait <= 0 {
			return nil
		}
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func (l *dbPhoneLimiter) Observe(phone string, status int, retryAfter time.Duration) {
	if status != 429 {
		return
	}
	now := time.Now()
	_ = l.db.Transaction(func(tx *gorm.DB) error {
		var b cloud_model.RateBucket
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("phone_number_id = ?", phone).First(&b).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			b = cloud_model.RateBucket{PhoneNumberID: phone, Tokens: 0, EffectiveRPS: l.baseRPS / 2, UpdatedAt: now}
		} else if err != nil {
			return err
		}
		b.Tokens = 0
		b.EffectiveRPS = math.Max(l.baseRPS/16, b.EffectiveRPS/2)
		b.Last429At = &now
		if retryAfter > 0 {
			until := now.Add(retryAfter)
			b.PausedUntil = &until
		}
		b.UpdatedAt = now
		return tx.Save(&b).Error
	})
}

func (l *dbPhoneLimiter) EffectiveRPS(phone string) float64 {
	var b cloud_model.RateBucket
	if err := l.db.Where("phone_number_id = ?", phone).First(&b).Error; err == nil && b.EffectiveRPS > 0 {
		return b.EffectiveRPS
	}
	return l.baseRPS
}
