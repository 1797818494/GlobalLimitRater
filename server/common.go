package server

import "time"

type RateLimitType int

const (
	FixedWindow   RateLimitType = iota // 固定窗口
	SlidingWindow                      // 滑动窗口
	LeakyBucket                        // 漏桶
	TokenBucket                        // 令牌桶
)

// 限流配置参数
type RateLimitConfig struct {
	Type        RateLimitType // 算法类型
	WindowSize  time.Duration // 窗口时间（滑动/固定窗口）
	MaxRequests int           // 窗口内最大请求数
	LeakRate    float64       // 漏桶流速（请求/秒）
	BucketSize  int           // 漏桶/令牌桶容量
	TokenRate   float64       // 令牌生成速率（令牌/秒）
	MaxWaitTime time.Duration
}
