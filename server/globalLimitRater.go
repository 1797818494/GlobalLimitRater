package server

import (
	"errors"
	"fmt"
	"log"
	"math/rand"
	"sync"
	"time"
)

type MetaService interface {
	Allow(acquire int) (bool, error)
}

// 为了兼容各种情况，目前按1s去周期性批量拉去
type GlobalLimitRater struct {
	metaService        MetaService
	localAllow         int
	fallBackAllow      int
	RequestForAllowNum int
	refreshInterval    time.Duration
	notifyRequestChan  chan int // Changed to int to carry the requested amount
	quitChan           chan struct{}
	mu                 sync.Mutex
}

func NewGlobalLimitRater(metaService MetaService, refreshInterval time.Duration) *GlobalLimitRater {
	if refreshInterval <= 0 {
		refreshInterval = time.Second // Default to 1 second if invalid
	}
	glr := &GlobalLimitRater{
		metaService:        metaService,
		localAllow:         0,
		refreshInterval:    refreshInterval,
		notifyRequestChan:  make(chan int),
		quitChan:           make(chan struct{}),
		fallBackAllow:      10,
		RequestForAllowNum: 100,
	}
	go glr.refreshLoop()
	return glr
}

func (globalLimitRater *GlobalLimitRater) Allow(acquire int) bool {
	if acquire <= 0 {
		return true // Allow zero or negative requests
	}

	globalLimitRater.mu.Lock()
	if globalLimitRater.localAllow >= acquire {
		globalLimitRater.localAllow -= acquire
		globalLimitRater.mu.Unlock()
		log.Println("扣减成功")
		return true
	}
	globalLimitRater.mu.Unlock()

	// Trigger a remote batch request if local allowance is insufficient
	globalLimitRater.notifyRequestChan <- acquire
	globalLimitRater.mu.Lock()
	log.Println("fail localAllow is ", globalLimitRater.localAllow)
	globalLimitRater.mu.Unlock()
	return false // Let the caller handle the blocking/retry logic
}

func (globalLimitRater *GlobalLimitRater) refreshLoop() {
	ticker := time.NewTicker(globalLimitRater.refreshInterval)
	defer ticker.Stop()

	for {
		globalLimitRater.mu.Lock()
		log.Println("cur localAllow is ", globalLimitRater.localAllow)
		globalLimitRater.mu.Unlock()
		select {
		case <-globalLimitRater.notifyRequestChan:
			globalLimitRater.TryAskForRequestNum(globalLimitRater.RequestForAllowNum)
		case <-ticker.C:
			globalLimitRater.mu.Lock()
			globalLimitRater.localAllow = globalLimitRater.fallBackAllow
			globalLimitRater.mu.Unlock()
			globalLimitRater.TryAskForRequestNum(globalLimitRater.RequestForAllowNum - globalLimitRater.fallBackAllow)
			log.Println("tick localAllow is ", globalLimitRater.localAllow)
		case <-globalLimitRater.quitChan:
			return
		}
	}
}

func (globalLimitRater *GlobalLimitRater) TryAskForRequestNum(requestForAllowNum int) (int, error) {
	if requestForAllowNum <= 0 {
		return 0, nil
	}
	allowed, err := globalLimitRater.metaService.Allow(requestForAllowNum)
	if err == nil && allowed {
		globalLimitRater.mu.Lock()
		globalLimitRater.localAllow += requestForAllowNum
		globalLimitRater.mu.Unlock()
		return requestForAllowNum, nil
	} else if err == nil && !allowed {
		//减低请求数重试？
		log.Println("限流")
		return 0, errors.New("触发全局限流")
	} else if err != nil {
		globalLimitRater.mu.Lock()
		if globalLimitRater.localAllow < globalLimitRater.fallBackAllow {
			globalLimitRater.localAllow = globalLimitRater.fallBackAllow
		}
		globalLimitRater.mu.Unlock()
		log.Println("网络问题")
		return globalLimitRater.fallBackAllow, err
	}
	log.Panic("不可能到达")
	return 0, nil
}

func (globalLimitRater *GlobalLimitRater) Stop() {
	close(globalLimitRater.quitChan)
}

// Mock MetaService for testing with simulated network errors
type MockMetaServiceWithRefresh struct {
	allowedPtr  *int
	mu          sync.Mutex
	errorRate   float64
	refreshDone chan struct{}
}

func NewMockMetaServiceWithRefresh(initialAllowed int, errorRate float64) *MockMetaServiceWithRefresh {
	allowed := initialAllowed
	m := &MockMetaServiceWithRefresh{
		allowedPtr:  &allowed,
		errorRate:   errorRate,
		refreshDone: make(chan struct{}),
	}
	go m.refreshAllowedPeriodically()
	return m
}

func (m *MockMetaServiceWithRefresh) Allow(acquire int) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	//Simulate network error with a probability
	if rand.Float64() < m.errorRate {
		return false, fmt.Errorf("network error")
	}

	if *m.allowedPtr >= acquire {
		*m.allowedPtr -= acquire
		return true, nil
	}
	return false, nil
}

func (m *MockMetaServiceWithRefresh) refreshAllowedPeriodically() {
	// 如果设计为1s可能错过第一次ticker更新， 导致后面需要某个请求第二次请求去主动拉去localCache, 导致失败
	ticker := time.NewTicker(time.Millisecond * 950)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			m.mu.Lock()
			*m.allowedPtr = 100 // Reset allowed count every second
			m.mu.Unlock()
		case <-m.refreshDone:
			return
		}
	}
}

func (m *MockMetaServiceWithRefresh) StopRefresh() {
	close(m.refreshDone)
}
