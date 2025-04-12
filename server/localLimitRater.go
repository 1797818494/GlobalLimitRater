package server

import (
	"log"
	"math"
	"sync"
	"time"
)

type LocalRateLimiter interface {
	Allow(acquire int) bool // 检查是否允许请求
}

// 固定窗口限流器实现
type fixedWindowLimiter struct {
	windowStart time.Time
	count       int
	max         int
	window      time.Duration
	mu          sync.Mutex
}

// 滑动窗口限流器实现
type slidingWindowLimiter struct {
	slots      []int         // 时间片计数器
	slotSize   time.Duration // 单个时间片长度
	windowSize time.Duration // 总窗口长度
	maxReq     int           // 最大请求数
	pos        int           // 当前时间片位置
	lastUpdate time.Time
	mu         sync.Mutex
}

// 漏桶限流器实现
type leakyBucketLimiter struct {
	capacity    float64   // 桶容量
	leakRate    float64   // 漏出速率（请求/秒）
	water       float64   // 当前水量
	lastLeak    time.Time // 上次漏水时间
	maxWaitTime time.Duration
	mu          sync.Mutex
}

// 令牌桶限流器实现
type tokenBucketLimiter struct {
	capacity   float64   // 桶容量
	rate       float64   // 令牌生成速率（令牌/秒）
	tokens     float64   // 当前令牌数
	lastRefill time.Time // 上次补充时间
	mu         sync.Mutex
}

func (f *fixedWindowLimiter) Allow(acquire int) bool {
	f.mu.Lock()
	defer f.mu.Unlock()

	now := time.Now()
	if now.Sub(f.windowStart) > f.window {
		f.windowStart = now
		f.count = 0
	}
	if f.count+acquire > f.max {
		return false
	}
	f.count += acquire
	return true
}

func (s *slidingWindowLimiter) Allow(acquire int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(s.lastUpdate)

	// 计算需要滑动的时间片数量
	slotsToMove := int(elapsed / s.slotSize)
	if slotsToMove > 0 {
		for i := 0; i < slotsToMove; i++ {
			s.pos = (s.pos + 1) % len(s.slots)
			s.slots[s.pos] = 0
		}
		s.lastUpdate = now.Add(-(elapsed % s.slotSize))
	}

	// 统计当前窗口总请求数
	total := 0
	for _, v := range s.slots {
		total += v
	}
	if total+acquire > s.maxReq {
		return false
	}
	for i := 0; i < acquire; i++ {
		s.slots[s.pos]++
		s.pos = (s.pos + 1) % len(s.slots)
	}
	return true
}

// 注意决定请求是否准入和请求的操作时间在漏桶概念里是分开的，我的实现里这里如果Allow是返回成功，在返回之前会进行同步等待
func (l *leakyBucketLimiter) Allow(acquire int) bool {
	l.mu.Lock()

	now := time.Now()

	// 计算漏出水量
	if !l.lastLeak.IsZero() {
		elapsed := now.Sub(l.lastLeak).Seconds()
		leaked := elapsed * l.leakRate
		l.water = math.Max(0, l.water-leaked)
	}
	l.lastLeak = now
	if l.water+float64(acquire) > l.capacity {
		l.mu.Unlock()
		return false
	}
	interval := time.Second / time.Duration(l.leakRate)
	wait := time.Duration(float64(l.water) * float64(interval))
	if wait > l.maxWaitTime {
		l.mu.Unlock()
		return false
	}
	l.water += float64(acquire)
	l.mu.Unlock()
	time.Sleep(wait)
	return true
}

func (t *tokenBucketLimiter) Allow(acquire int) bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(t.lastRefill).Seconds()

	// 补充令牌
	newTokens := elapsed * t.rate
	t.tokens = math.Min(t.capacity, t.tokens+newTokens)
	t.lastRefill = now

	if t.tokens >= float64(acquire) {
		t.tokens -= float64(acquire)
		return true
	}
	return false
}

func NewLocalRateLimit(config RateLimitConfig) LocalRateLimiter {

	switch config.Type {
	case FixedWindow:
		return &fixedWindowLimiter{
			max:    config.MaxRequests,
			window: config.WindowSize,
		}
	case SlidingWindow:
		slotNum := int(config.WindowSize / (time.Second / 10)) // 每个时间片100ms
		return &slidingWindowLimiter{
			slots:      make([]int, slotNum),
			slotSize:   time.Second / 10,
			windowSize: config.WindowSize,
			maxReq:     config.MaxRequests,
			lastUpdate: time.Now(),
		}
	case LeakyBucket:
		return &leakyBucketLimiter{
			capacity: float64(config.BucketSize),
			leakRate: config.LeakRate,
		}
	case TokenBucket:
		return &tokenBucketLimiter{
			capacity:   float64(config.BucketSize),
			rate:       config.TokenRate,
			tokens:     float64(config.BucketSize), // 初始满令牌
			lastRefill: time.Now(),
		}
	default:
		log.Fatal("not support rateLimiter type")
		return nil
	}
}
