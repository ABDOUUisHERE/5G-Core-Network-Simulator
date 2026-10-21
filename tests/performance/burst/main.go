// Burst measures a synchronized registration workload without one OS thread per UE.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

type outcome struct {
	latency float64
	err     string
}

func percentile(values []float64, p float64) any {
	if len(values) == 0 {
		return nil
	}
	sort.Float64s(values)
	index := int(math.Ceil(float64(len(values))*p)) - 1
	if index < 0 {
		index = 0
	}
	return values[index]
}
func main() {
	base := flag.String("base", "http://127.0.0.1:8085", "control API URL")
	count := flag.Int("count", 1000, "new synthetic UE registrations (1..1000)")
	concurrency := flag.Int("concurrency", 1000, "maximum simultaneous requests (1..1000)")
	output := flag.String("output", "test-results/burst.json", "measured report file")
	flag.Parse()
	if *count < 1 || *count > 1000 || *concurrency < 1 || *concurrency > 1000 {
		fmt.Fprintln(os.Stderr, "count/concurrency must be 1..1000")
		os.Exit(2)
	}
	client := &http.Client{Timeout: 12 * time.Second, Transport: &http.Transport{MaxConnsPerHost: *concurrency, MaxIdleConnsPerHost: *concurrency, MaxIdleConns: *concurrency, IdleConnTimeout: 30 * time.Second}}
	defer client.CloseIdleConnections()
	start := make(chan struct{})
	semaphore := make(chan struct{}, *concurrency)
	results := make(chan outcome, *count)
	var wg sync.WaitGroup
	var active, peak atomic.Int64
	for i := 1; i <= *count; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			<-start
			semaphore <- struct{}{}
			defer func() { <-semaphore }()
			payload, _ := json.Marshal(map[string]string{"supi": fmt.Sprintf("imsi-00101%010d", index), "credential": "demo-key", "gnb_id": "gnb-burst-01"})
			request, _ := http.NewRequest(http.MethodPost, *base+"/api/ues", bytes.NewReader(payload))
			request.Header.Set("Content-Type", "application/json")
			now := active.Add(1)
			for previous := peak.Load(); now > previous && !peak.CompareAndSwap(previous, now); previous = peak.Load() {
			}
			began := time.Now()
			response, err := client.Do(request)
			item := outcome{}
			if err != nil {
				item.err = err.Error()
			} else {
				data, readErr := io.ReadAll(response.Body)
				response.Body.Close()
				if readErr != nil {
					item.err = readErr.Error()
				} else if response.StatusCode != 200 {
					item.err = fmt.Sprintf("HTTP %d: %.160s", response.StatusCode, data)
				} else {
					var decoded struct {
						Registration struct {
							Accepted bool `json:"accepted"`
						} `json:"registration"`
					}
					if err = json.Unmarshal(data, &decoded); err != nil {
						item.err = err.Error()
					} else if !decoded.Registration.Accepted {
						item.err = "registration refused"
					}
				}
			}
			item.latency = float64(time.Since(began).Microseconds()) / 1000
			active.Add(-1)
			results <- item
		}(i)
	}
	began := time.Now()
	close(start)
	wg.Wait()
	close(results)
	all, successful := []float64{}, []float64{}
	errors := []string{}
	errorCount := 0
	for result := range results {
		all = append(all, result.latency)
		if result.err == "" {
			successful = append(successful, result.latency)
		} else {
			errorCount++
			if len(errors) < 10 {
				errors = append(errors, result.err)
			}
		}
	}
	wall := time.Since(began).Seconds()
	healthResponse, healthErr := client.Get(*base + "/readyz")
	recovered := false
	if healthErr == nil {
		recovered = healthResponse.StatusCode == 200
		healthResponse.Body.Close()
	}
	report := map[string]any{"timestamp_utc": time.Now().UTC(), "count": *count, "concurrency_limit": *concurrency, "observed_peak_inflight": peak.Load(), "successes": len(successful), "errors": errorCount, "error_samples": errors, "wall_seconds": wall, "throughput_per_second": float64(*count) / wall, "all_p50_ms": percentile(all, .5), "all_p95_ms": percentile(all, .95), "all_p99_ms": percentile(all, .99), "success_p50_ms": percentile(successful, .5), "success_p95_ms": percentile(successful, .95), "success_p99_ms": percentile(successful, .99), "ready_after_load": recovered, "generator": map[string]any{"go": runtime.Version(), "os": runtime.GOOS, "arch": runtime.GOARCH, "logical_cpus": runtime.NumCPU()}, "method": "all goroutines released from one start gate; active count measures actual HTTP attempts, not waiting goroutines; no retry; same runner/stack as the adjacent baseline report"}
	data, _ := json.MarshalIndent(report, "", "  ")
	if err := os.MkdirAll(filepath.Dir(*output), 0755); err != nil {
		panic(err)
	}
	if err := os.WriteFile(*output, append(data, '\n'), 0644); err != nil {
		panic(err)
	}
	fmt.Println(string(data))
	if !recovered {
		fmt.Fprintln(os.Stderr, "control API did not recover after the workload")
		os.Exit(1)
	}
}
