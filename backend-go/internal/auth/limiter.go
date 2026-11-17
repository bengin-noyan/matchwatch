package auth

import (
	"sync"
	"time"
)

// Limiter, başarısız girişleri anahtara (e-posta) göre kayan bir zaman penceresinde sayar.
// shortcut: sayaç bellekte tutuluyor, tek API örneği için doğru; API birden çok kopya çalışınca Postgres'e taşı.
type Limiter struct {
	mu     sync.Mutex
	max    int
	window time.Duration
	fails  map[string][]time.Time
	now    func() time.Time
}

func NewLimiter(max int, window time.Duration) *Limiter {
	return &Limiter{max: max, window: window, fails: map[string][]time.Time{}, now: time.Now}
}

func (l *Limiter) recent(key string) []time.Time {
	cut := l.now().Add(-l.window)
	kept := l.fails[key][:0]
	for _, t := range l.fails[key] {
		if t.After(cut) {
			kept = append(kept, t)
		}
	}
	if len(kept) == 0 {
		delete(l.fails, key)
		return nil
	}
	l.fails[key] = kept
	return kept
}

// Blocked, anahtarın penceredeki başarısız deneme hakkını bitirip bitirmediğini söyler.
func (l *Limiter) Blocked(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.recent(key)) >= l.max
}

func (l *Limiter) Fail(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.fails[key] = append(l.recent(key), l.now())
}

func (l *Limiter) Reset(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.fails, key)
}

// PasswordCost, saklanan şifrelerin bcrypt maliyeti (cmd/onboard). Girişte zamanlamayı eşitlemek için de kullanılır.
const PasswordCost = 12
