package server

import (
	"context"
	"log"
	"runtime"
	"runtime/debug"
	"sync"

	"github.com/Parsaeffatravesh/tragge/packages/observability"
)

// ====================
// Broadcast Worker Pool
// ====================

// BroadcastJob represents a job for the broadcast worker pool
type BroadcastJob struct {
	client      *Client
	data        []byte // For legacy SendMessage calls
	jsonData    []byte // JSON encoded data for tick_batch/state_delta
	msgpackData []byte // MessagePack encoded data for tick_batch/state_delta
	msgType     string // "tick_batch", "state_delta", or empty for legacy
}

// BroadcastWorkerPool manages a pool of goroutines for broadcasting messages
type BroadcastWorkerPool struct {
	jobs       chan BroadcastJob
	numWorkers int
	wg         sync.WaitGroup
	ctx        context.Context
	cancel     context.CancelFunc
}

// NewBroadcastWorkerPool creates a new worker pool with the specified number of workers
func NewBroadcastWorkerPool(numWorkers int) *BroadcastWorkerPool {
	if numWorkers <= 0 {
		numWorkers = runtime.NumCPU() * 2 // Default: 2 workers per CPU
	}

	ctx, cancel := context.WithCancel(context.Background())
	pool := &BroadcastWorkerPool{
		jobs:       make(chan BroadcastJob, numWorkers*100), // Buffered for burst handling
		numWorkers: numWorkers,
		ctx:        ctx,
		cancel:     cancel,
	}

	// Start workers
	for i := 0; i < numWorkers; i++ {
		pool.wg.Add(1)
		go pool.worker()
	}

	return pool
}

// worker processes jobs from the queue
func (p *BroadcastWorkerPool) worker() {
	defer p.wg.Done()

	for {
		select {
		case <-p.ctx.Done():
			return
		case job, ok := <-p.jobs:
			if !ok {
				return
			}
			// Handle encoding-aware message types
			switch job.msgType {
			case "tick_batch":
				job.client.SendTickBatch(job.jsonData, job.msgpackData)
			case "state_delta":
				job.client.SendStateDelta(job.jsonData, job.msgpackData)
			case "critical":
				job.client.SendCriticalMessage(job.data)
			default:
				// Legacy message
				job.client.SendMessage(job.data)
			}
		}
	}
}

// Submit adds a broadcast job to the worker pool
func (p *BroadcastWorkerPool) Submit(client *Client, data []byte) {
	select {
	case p.jobs <- BroadcastJob{client: client, data: data}:
		// Job submitted
	default:
		// Queue full - job dropped (client will get next update)
		// This is acceptable for real-time data as we prefer fresh data over stale queued data
	}
}

// SubmitTickBatch adds a tick_batch broadcast job with encoding-aware data
func (p *BroadcastWorkerPool) SubmitTickBatch(client *Client, jsonData, msgpackData []byte) {
	select {
	case p.jobs <- BroadcastJob{client: client, jsonData: jsonData, msgpackData: msgpackData, msgType: "tick_batch"}:
		// Job submitted
	default:
		// Queue full - job dropped
	}
}

// SubmitStateDelta adds a state_delta broadcast job with encoding-aware data
func (p *BroadcastWorkerPool) SubmitStateDelta(client *Client, jsonData, msgpackData []byte) {
	select {
	case p.jobs <- BroadcastJob{client: client, jsonData: jsonData, msgpackData: msgpackData, msgType: "state_delta"}:
		// Job submitted
	default:
		// Queue full - job dropped
	}
}

// BroadcastToAll sends a message to multiple clients using the worker pool
func (p *BroadcastWorkerPool) BroadcastToAll(clients []*Client, data []byte) {
	for _, client := range clients {
		p.Submit(client, data)
	}
}

// BroadcastTickBatchToAll sends a tick_batch message to multiple clients with encoding-aware serialization
func (p *BroadcastWorkerPool) BroadcastTickBatchToAll(clients []*Client, jsonData, msgpackData []byte) {
	for _, client := range clients {
		p.SubmitTickBatch(client, jsonData, msgpackData)
	}
}

// SubmitCritical adds a critical message job to the worker pool.
// If the job queue is full, it sends directly in a goroutine to avoid dropping critical messages.
func (p *BroadcastWorkerPool) SubmitCritical(client *Client, data []byte) {
	select {
	case p.jobs <- BroadcastJob{client: client, data: data, msgType: "critical"}:
		// Job submitted
	default:
		// Queue full - send directly in a goroutine to avoid dropping critical messages
		go func() {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("panic in SendCriticalMessage goroutine: %s\n%s", observability.RedactPanic(r), observability.RedactText(string(debug.Stack())))
				}
			}()
			client.SendCriticalMessage(data)
		}()
	}
}

// BroadcastCriticalToAll sends a critical message to multiple clients using the worker pool
func (p *BroadcastWorkerPool) BroadcastCriticalToAll(clients []*Client, data []byte) {
	for _, client := range clients {
		p.SubmitCritical(client, data)
	}
}

// Stop shuts down the worker pool
func (p *BroadcastWorkerPool) Stop() {
	p.cancel()
	p.wg.Wait() // Wait for workers to exit before closing the channel
	close(p.jobs)
}
