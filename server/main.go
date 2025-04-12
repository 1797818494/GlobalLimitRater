package server

// func main() {
// 	// Create a mock MetaService with network error simulation
// 	mockMetaService := &MockMetaServiceWithRefresh{allowed: 100, errorRate: 0.1}

// 	// Initialize the GlobalLimitRater
// 	rater := NewGlobalLimitRater(mockMetaService, time.Second)
// 	defer rater.Stop()

// 	var wg sync.WaitGroup
// 	var allowedCount int64
// 	startTime := time.Now()
// 	ticker := time.NewTicker(time.Second)
// 	defer ticker.Stop()
// 	var lastSecondAllowed int64

// 	go func() {
// 		for range ticker.C {
// 			currentAllowed := atomic.LoadInt64(&allowedCount)
// 			fmt.Printf("Throughput per second: %d\n", currentAllowed-lastSecondAllowed)
// 			lastSecondAllowed = currentAllowed
// 		}
// 	}()

// 	for i := 0; i < 150; i++ {
// 		wg.Add(1)
// 		go func() {
// 			defer wg.Done()
// 			if rater.Allow(1) {
// 				atomic.AddInt64(&allowedCount, 1)
// 				println("Request allowed")
// 			} else {
// 				println("Request blocked")
// 				time.Sleep(100 * time.Millisecond) // Simulate backoff
// 				if rater.Allow(1) {
// 					atomic.AddInt64(&allowedCount, 1)
// 					println("Request allowed after retry")
// 				} else {
// 					println("Request still blocked")
// 				}
// 			}
// 			time.Sleep(50 * time.Millisecond)
// 		}()
// 	}
// 	wg.Wait()

// 	elapsed := time.Since(startTime)
// 	fmt.Printf("Total allowed requests: %d in %s\n", atomic.LoadInt64(&allowedCount), elapsed)
// }
