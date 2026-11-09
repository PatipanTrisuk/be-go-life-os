package middleware

import (
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)

type RateLimitConfig struct {
	PerMin       float64
	Burst        int
	SkipPrefixes []string
}

type visitor struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

type IPRateLimiter struct {
	mu       sync.Mutex
	visitors map[string]*visitor
	limit    rate.Limit
	burst    int
}

func NewIPRateLimiter(perMin float64, burst int) *IPRateLimiter {
	if perMin <= 0 {
		perMin = 60
	}
	if burst < 1 {
		burst = 1
	}

	limiter := &IPRateLimiter{
		visitors: make(map[string]*visitor),
		limit:    rate.Limit(perMin / 60.0),
		burst:    burst,
	}
	go limiter.cleanupLoop()
	return limiter
}

func (l *IPRateLimiter) getVisitor(ip string) *rate.Limiter {
	l.mu.Lock()
	defer l.mu.Unlock()

	v, exists := l.visitors[ip]
	if !exists {
		limiter := rate.NewLimiter(l.limit, l.burst)
		l.visitors[ip] = &visitor{limiter: limiter, lastSeen: time.Now()}
		return limiter
	}

	v.lastSeen = time.Now()
	return v.limiter
}

func (l *IPRateLimiter) cleanupLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		l.mu.Lock()
		for ip, v := range l.visitors {
			if time.Since(v.lastSeen) > 10*time.Minute {
				delete(l.visitors, ip)
			}
		}
		l.mu.Unlock()
	}
}

func RateLimit(cfg RateLimitConfig) gin.HandlerFunc {
	limiter := NewIPRateLimiter(cfg.PerMin, cfg.Burst)

	return func(c *gin.Context) {
		path := c.Request.URL.Path
		for _, prefix := range cfg.SkipPrefixes {
			if strings.HasPrefix(path, prefix) {
				c.Next()
				return
			}
		}

		ip := c.ClientIP()
		visitorLimiter := limiter.getVisitor(ip)
		if visitorLimiter.Allow() {
			c.Next()
			return
		}

		retryAfter := retryAfterSeconds(visitorLimiter)
		c.Header("Retry-After", strconv.Itoa(retryAfter))
		c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
			"statusCode": http.StatusTooManyRequests,
			"code":       "TOO_MANY_REQUESTS",
			"message":    rateLimitMessage(c),
		})
	}
}

func retryAfterSeconds(limiter *rate.Limiter) int {
	reservation := limiter.Reserve()
	delay := reservation.Delay()
	reservation.Cancel()

	seconds := int(math.Ceil(delay.Seconds()))
	if seconds < 1 {
		return 1
	}
	return seconds
}

func rateLimitMessage(c *gin.Context) string {
	if v, ok := c.Get(LocaleKey); ok {
		if lang, ok := v.(string); ok && lang == "th" {
			return "คำขอมากเกินไป กรุณาลองใหม่ภายหลัง"
		}
	}
	return "Too many requests. Please try again later."
}

func envFloat(key string, fallback float64) float64 {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	n, err := strconv.ParseFloat(raw, 64)
	if err != nil || n <= 0 {
		return fallback
	}
	return n
}

func envInt(key string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 {
		return fallback
	}
	return n
}

func GlobalRateLimitConfig() RateLimitConfig {
	return RateLimitConfig{
		PerMin:       envFloat("RATE_LIMIT_PER_MIN", 300),
		Burst:        envInt("RATE_LIMIT_BURST", 80),
		SkipPrefixes: []string{"/api/health"},
	}
}

func AuthRateLimitConfig() RateLimitConfig {
	return RateLimitConfig{
		PerMin: envFloat("AUTH_RATE_LIMIT_PER_MIN", 10),
		Burst:  envInt("AUTH_RATE_LIMIT_BURST", 5),
	}
}
