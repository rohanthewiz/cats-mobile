package catsclient

import (
	"math/rand/v2"
	"time"
)

// Backoff is the reconnect ladder: 500 ms doubling to 30 s, with ±20 % jitter.
//
// The jitter is not decoration. Every phone on a network that dropped comes
// back at the same moment, and an unjittered ladder has them all knocking in
// lockstep, which is how a home server that just restarted gets a thundering
// herd instead of a queue.
//
// Not safe for concurrent use; one reconnect loop owns one Backoff.
type Backoff struct {
	Initial time.Duration
	Ceiling time.Duration

	rand    *rand.Rand
	attempt int
}

// NewBackoff returns the default ladder. A nil source uses the global one; a
// test passes a seeded rand.New(rand.NewPCG(...)) for repeatability.
func NewBackoff(r *rand.Rand) *Backoff {
	return &Backoff{Initial: 500 * time.Millisecond, Ceiling: 30 * time.Second, rand: r}
}

// Reset returns to the bottom of the ladder. Call on a successful connect, on
// resume, on a network change, and on pull-to-refresh. A user who pulled to
// refresh is saying they think it should work now; making them wait out a 30 s
// ladder is answering an explicit request with a shrug.
func (b *Backoff) Reset() { b.attempt = 0 }

// Next is how long to wait before the next attempt, and advances the ladder.
func (b *Backoff) Next() time.Duration {
	// The shift is clamped so a connection that has been failing for hours
	// cannot overflow the multiplier; the ceiling makes anything past 2^20
	// irrelevant anyway.
	shift := min(b.attempt, 20)
	scaled := b.Initial * (1 << shift)
	capped := min(scaled, b.Ceiling)
	b.attempt++
	jitter := 1.0 + (b.float()*0.4 - 0.2)
	return time.Duration(float64(capped) * jitter)
}

func (b *Backoff) float() float64 {
	if b.rand != nil {
		return b.rand.Float64()
	}
	return rand.Float64()
}
