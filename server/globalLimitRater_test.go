package server

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestGlobalLimitRaterWithRefresh(t *testing.T) {
	// Create a mock MetaService with dynamic allowed count and error simulation
	mockMetaService := NewMockMetaServiceWithRefresh(100, 0.1)
	defer mockMetaService.StopRefresh()

	// Initialize the GlobalLimitRater
	rater := NewGlobalLimitRater(mockMetaService, time.Second)
	defer rater.Stop()

	var wg sync.WaitGroup
	var allowedCount int64
	startTime := time.Now()
	throughputTicker := time.NewTicker(time.Second)
	defer throughputTicker.Stop()
	var lastSecondAllowed int64

	go func() {
		for range throughputTicker.C {
			currentAllowed := atomic.LoadInt64(&allowedCount)
			fmt.Printf("Throughput per second: %d\n", currentAllowed-lastSecondAllowed)
			lastSecondAllowed = currentAllowed
		}
	}()

	numRequests := 150
	for i := 0; i < numRequests; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			//	println("Request start, ", i)
			if rater.Allow(1) {
				atomic.AddInt64(&allowedCount, 1)
				// println("Request allowed")
			} else {
				// println("Request blocked")
				time.Sleep(1500 * time.Millisecond) // Simulate backoff
				if rater.Allow(1) {
					atomic.AddInt64(&allowedCount, 1)
					// println("Request allowed after retry")
				} else {
					println("Request still blocked, ", i)
				}
			}
			time.Sleep(50 * time.Millisecond)
		}(i)
	}
	wg.Wait()

	elapsed := time.Since(startTime)
	fmt.Printf("Total allowed requests: %d in %s\n", atomic.LoadInt64(&allowedCount), elapsed)
}
