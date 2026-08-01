package server

import (
	"math"
	"sync/atomic"
)

// ====================
// Metrics
// ====================

// Metrics holds atomic counters for WebSocket metrics
type Metrics struct {
	wsConnections          atomic.Int64
	wsDroppedMessagesTotal atomic.Int64
	wsBroadcastCount       atomic.Int64
	wsBroadcastTotalMs     atomic.Int64
	wsBroadcastMinMs       atomic.Int64
	wsBroadcastMaxMs       atomic.Int64

	// Compression metrics
	wsCompressedConnections   atomic.Int64 // Number of connections with compression negotiated
	wsUncompressedConnections atomic.Int64 // Number of connections without compression
	wsTotalBytesSent          atomic.Int64 // Total uncompressed bytes (before compression)
	wsCompressedBytesSent     atomic.Int64 // Total compressed bytes (after compression, estimated)
	wsCompressionSkipped      atomic.Int64 // Messages skipped compression (below threshold)

	// Batching metrics
	wsBandwidthSaved        atomic.Int64 // Bytes saved through message batching
	wsBatchMessagesSaved    atomic.Int64 // Number of individual messages avoided by batching
	wsDeltaCompressionRatio atomic.Int64 // Delta compression ratio * 100 (for atomic storage)

	// MessagePack encoding metrics
	wsMsgPackConnections atomic.Int64 // Number of connections using MessagePack encoding
	wsJsonConnections    atomic.Int64 // Number of connections using JSON encoding (default)
	// Bytes sent counters by encoding and message type (for bandwidth comparison)
	wsBytesSentJsonTickBatch     atomic.Int64 // Bytes sent for tick_batch via JSON
	wsBytesSentMsgPackTickBatch  atomic.Int64 // Bytes sent for tick_batch via MessagePack
	wsBytesSentJsonStateDelta    atomic.Int64 // Bytes sent for state_delta via JSON
	wsBytesSentMsgPackStateDelta atomic.Int64 // Bytes sent for state_delta via MessagePack
	wsBytesSentJsonOther         atomic.Int64 // Bytes sent for other messages via JSON

	// Critical message metrics
	wsCriticalMessagesSent atomic.Int64 // Total critical messages successfully queued
	wsCriticalQueueFull    atomic.Int64 // Critical queue full events (timeout → connection closed)
}

// NewMetrics creates a new Metrics instance
func NewMetrics() *Metrics {
	m := &Metrics{}
	m.wsBroadcastMinMs.Store(math.MaxInt64)
	return m
}

// RecordBroadcast records a broadcast duration
func (m *Metrics) RecordBroadcast(durationMs int64) {
	m.wsBroadcastCount.Add(1)
	m.wsBroadcastTotalMs.Add(durationMs)

	// Update min
	for {
		oldMin := m.wsBroadcastMinMs.Load()
		if durationMs >= oldMin || m.wsBroadcastMinMs.CompareAndSwap(oldMin, durationMs) {
			break
		}
	}

	// Update max
	for {
		oldMax := m.wsBroadcastMaxMs.Load()
		if durationMs <= oldMax || m.wsBroadcastMaxMs.CompareAndSwap(oldMax, durationMs) {
			break
		}
	}
}

// GetStats returns current metrics as a map
func (m *Metrics) GetStats() map[string]interface{} {
	count := m.wsBroadcastCount.Load()
	totalMs := m.wsBroadcastTotalMs.Load()
	avgMs := float64(0)
	if count > 0 {
		avgMs = float64(totalMs) / float64(count)
	}

	minMs := m.wsBroadcastMinMs.Load()
	if minMs == math.MaxInt64 {
		minMs = 0
	}

	// Calculate compression ratio
	totalBytes := m.wsTotalBytesSent.Load()
	compressedBytes := m.wsCompressedBytesSent.Load()
	compressionRatio := float64(0)
	if totalBytes > 0 && compressedBytes > 0 {
		compressionRatio = 1.0 - (float64(compressedBytes) / float64(totalBytes))
	}

	// Calculate MessagePack bandwidth savings
	jsonTickBytes := m.wsBytesSentJsonTickBatch.Load()
	msgpackTickBytes := m.wsBytesSentMsgPackTickBatch.Load()
	tickBandwidthSavings := float64(0)
	if jsonTickBytes > 0 && msgpackTickBytes > 0 {
		// Compare average bytes per message (rough estimate based on total bytes)
		tickBandwidthSavings = 1.0 - (float64(msgpackTickBytes) / float64(jsonTickBytes))
	}

	jsonStateBytes := m.wsBytesSentJsonStateDelta.Load()
	msgpackStateBytes := m.wsBytesSentMsgPackStateDelta.Load()
	stateBandwidthSavings := float64(0)
	if jsonStateBytes > 0 && msgpackStateBytes > 0 {
		stateBandwidthSavings = 1.0 - (float64(msgpackStateBytes) / float64(jsonStateBytes))
	}

	return map[string]interface{}{
		"ws_connections":              m.wsConnections.Load(),
		"ws_dropped_messages_total":   m.wsDroppedMessagesTotal.Load(),
		"ws_broadcast_count":          count,
		"ws_broadcast_avg_ms":         avgMs,
		"ws_broadcast_min_ms":         minMs,
		"ws_broadcast_max_ms":         m.wsBroadcastMaxMs.Load(),
		"ws_compressed_connections":   m.wsCompressedConnections.Load(),
		"ws_uncompressed_connections": m.wsUncompressedConnections.Load(),
		"ws_total_bytes_sent":         totalBytes,
		"ws_compressed_bytes_sent":    compressedBytes,
		"ws_compression_ratio":        compressionRatio,
		"ws_compression_skipped":      m.wsCompressionSkipped.Load(),
		// Batching metrics
		"ws_bandwidth_saved_bytes":   m.wsBandwidthSaved.Load(),
		"ws_batch_messages_saved":    m.wsBatchMessagesSaved.Load(),
		"ws_delta_compression_ratio": float64(m.wsDeltaCompressionRatio.Load()) / 100.0,
		// MessagePack encoding metrics
		"ws_msgpack_connections":             m.wsMsgPackConnections.Load(),
		"ws_json_connections":                m.wsJsonConnections.Load(),
		"ws_bytes_sent_json_tick_batch":      jsonTickBytes,
		"ws_bytes_sent_msgpack_tick_batch":   msgpackTickBytes,
		"ws_bytes_sent_json_state_delta":     jsonStateBytes,
		"ws_bytes_sent_msgpack_state_delta":  msgpackStateBytes,
		"ws_bytes_sent_json_other":           m.wsBytesSentJsonOther.Load(),
		"ws_tick_batch_bandwidth_savings":    tickBandwidthSavings,  // Ratio of bandwidth saved with MessagePack
		"ws_state_delta_bandwidth_savings":   stateBandwidthSavings, // Ratio of bandwidth saved with MessagePack
		// Critical message metrics
		"ws_critical_messages_sent": m.wsCriticalMessagesSent.Load(),
		"ws_critical_queue_full":    m.wsCriticalQueueFull.Load(),
	}
}
