package access

import (
	"testing"
	"time"

	"github.com/tr4m0ryp/oido-google-calander/internal/config"
)

type clock struct{ t time.Time }

func (c *clock) now() time.Time          { return c.t }
func (c *clock) advance(d time.Duration) { c.t = c.t.Add(d) }

var testLimits = config.LoginLimits{MaxFailures: 5, Window: 15 * time.Minute,
	Lockout: 15 * time.Minute, GlobalMaxFailures: 50}

func TestLimiterLocksAfterMaxFailures(t *testing.T) {
	c := &clock{t: time.Unix(1_000_000, 0)}
	l := NewLimiter(testLimits, c.now)
	for i := 1; i <= 4; i++ {
		s := l.Failure("1.1.1.1")
		if s.Blocked || s.Remaining != 5-i {
			t.Fatalf("failure %d: %+v", i, s)
		}
	}
	s := l.Failure("1.1.1.1")
	if !s.Blocked || s.RetryAfter != 15*time.Minute {
		t.Fatalf("fifth failure: %+v", s)
	}
	if l.Check("2.2.2.2").Blocked {
		t.Error("another IP must not be locked")
	}
	c.advance(14 * time.Minute)
	if !l.Check("1.1.1.1").Blocked {
		t.Error("still inside lockout")
	}
	c.advance(time.Minute)
	if s := l.Check("1.1.1.1"); s.Blocked || s.Remaining != 5 {
		t.Errorf("after lockout: %+v", s)
	}
}

func TestLimiterWindowSlides(t *testing.T) {
	c := &clock{t: time.Unix(1_000_000, 0)}
	l := NewLimiter(testLimits, c.now)
	for i := 0; i < 4; i++ {
		l.Failure("ip")
	}
	c.advance(16 * time.Minute)
	if s := l.Failure("ip"); s.Blocked || s.Remaining != 4 {
		t.Errorf("old failures should have expired: %+v", s)
	}
}

func TestLimiterSuccessResets(t *testing.T) {
	l := NewLimiter(testLimits, nil)
	for i := 0; i < 4; i++ {
		l.Failure("ip")
	}
	l.Success("ip")
	if s := l.Check("ip"); s.Remaining != 5 {
		t.Errorf("after success: %+v", s)
	}
}

func TestLimiterGlobalCap(t *testing.T) {
	c := &clock{t: time.Unix(1_000_000, 0)}
	limits := testLimits
	limits.GlobalMaxFailures = 3
	l := NewLimiter(limits, c.now)
	l.Failure("a")
	l.Failure("b")
	l.Failure("c")
	if s := l.Check("fresh"); !s.Blocked {
		t.Fatalf("global cap not applied: %+v", s)
	}
	c.advance(15*time.Minute + time.Second)
	if l.Check("fresh").Blocked {
		t.Error("global block should expire with the window")
	}
}
