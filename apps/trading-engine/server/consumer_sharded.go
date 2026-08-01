package server

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Parsaeffatravesh/tragge/packages/infra"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kgo"
	"go.uber.org/zap"
)

// ShardedConsumer is a Kafka consumer that only consumes from specific partitions
// assigned to a particular shard. This enables partition-aware consumption for
// horizontally scaled services.
type ShardedConsumer struct {
	shardID       int
	shardCount    int
	client        *kgo.Client
	partitions    []int32
	topic         string
	brokers       []string
	consumerGroup string
	log           *zap.Logger

	// State
	running atomic.Bool
	mu      sync.RWMutex
	ctx     context.Context
	cancel  context.CancelFunc

	// Metrics
	metrics *ShardedConsumerMetrics
}

// ShardedConsumerConfig holds configuration for creating a ShardedConsumer.
type ShardedConsumerConfig struct {
	ShardID         int
	ShardCount      int
	TotalPartitions int // e.g., 16
	Brokers         []string
	Topic           string
	ConsumerGroup   string
	Logger          *zap.Logger

	// Kafka client options
	DisableAutoCommit bool
	SessionTimeout    time.Duration
	RebalanceTimeout  time.Duration
}

// ShardedConsumerMetrics holds Prometheus metrics for the sharded consumer.
type ShardedConsumerMetrics struct {
	// Partition assignment
	AssignedPartitions prometheus.Gauge

	// Consumer lag per partition
	ConsumerLag *prometheus.GaugeVec

	// Processing metrics
	MessagesProcessed *prometheus.CounterVec
	ProcessingLatency *prometheus.HistogramVec

	// Offset metrics
	CurrentOffset   *prometheus.GaugeVec
	HighWatermark   *prometheus.GaugeVec
	CommittedOffset *prometheus.GaugeVec

	// Error metrics
	FetchErrors   prometheus.Counter
	CommitErrors  prometheus.Counter
	ProcessErrors prometheus.Counter
}

// NewShardedConsumerMetrics creates and registers metrics for the sharded consumer.
func NewShardedConsumerMetrics(registry prometheus.Registerer, namespace string) *ShardedConsumerMetrics {
	metrics := &ShardedConsumerMetrics{
		AssignedPartitions: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "consumer_assigned_partitions",
			Help:      "Number of partitions assigned to this consumer",
		}),
		ConsumerLag: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "consumer_lag",
			Help:      "Consumer lag per partition (high watermark - committed offset)",
		}, []string{"topic", "partition"}),
		MessagesProcessed: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "consumer_messages_processed_total",
			Help:      "Total number of messages processed per partition",
		}, []string{"topic", "partition"}),
		ProcessingLatency: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "consumer_processing_latency_seconds",
			Help:      "Message processing latency in seconds",
			Buckets:   []float64{.001, .005, .01, .025, .05, .1, .25, .5, 1},
		}, []string{"topic", "partition"}),
		CurrentOffset: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "consumer_current_offset",
			Help:      "Current offset being processed per partition",
		}, []string{"topic", "partition"}),
		HighWatermark: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "consumer_high_watermark",
			Help:      "High watermark (latest offset) per partition",
		}, []string{"topic", "partition"}),
		CommittedOffset: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "consumer_committed_offset",
			Help:      "Last committed offset per partition",
		}, []string{"topic", "partition"}),
		FetchErrors: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "consumer_fetch_errors_total",
			Help:      "Total number of fetch errors",
		}),
		CommitErrors: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "consumer_commit_errors_total",
			Help:      "Total number of commit errors",
		}),
		ProcessErrors: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "consumer_process_errors_total",
			Help:      "Total number of processing errors",
		}),
	}

	// Register all metrics
	registry.MustRegister(
		metrics.AssignedPartitions,
		metrics.ConsumerLag,
		metrics.MessagesProcessed,
		metrics.ProcessingLatency,
		metrics.CurrentOffset,
		metrics.HighWatermark,
		metrics.CommittedOffset,
		metrics.FetchErrors,
		metrics.CommitErrors,
		metrics.ProcessErrors,
	)

	return metrics
}

// GetPartitionsForShard calculates which partitions belong to a shard.
// Uses modulo-based assignment: partition P belongs to shard P % shardCount.
//
// Example: 16 partitions, 4 shards
// Shard 0: [0, 4, 8, 12]
// Shard 1: [1, 5, 9, 13]
// Shard 2: [2, 6, 10, 14]
// Shard 3: [3, 7, 11, 15]
func GetPartitionsForShard(shardID, shardCount, totalPartitions int) []int32 {
	if shardCount <= 0 || totalPartitions <= 0 {
		return nil
	}
	if shardID < 0 || shardID >= shardCount {
		return nil
	}

	partitions := make([]int32, 0, totalPartitions/shardCount+1)
	for p := 0; p < totalPartitions; p++ {
		if p%shardCount == shardID {
			partitions = append(partitions, int32(p))
		}
	}
	return partitions
}

// NewShardedConsumer creates a new partition-aware Kafka consumer.
func NewShardedConsumer(cfg ShardedConsumerConfig) (*ShardedConsumer, error) {
	if cfg.ShardCount <= 0 {
		return nil, fmt.Errorf("shard count must be positive, got %d", cfg.ShardCount)
	}
	if cfg.ShardID < 0 || cfg.ShardID >= cfg.ShardCount {
		return nil, fmt.Errorf("shard ID %d out of range [0, %d)", cfg.ShardID, cfg.ShardCount)
	}
	if cfg.TotalPartitions <= 0 {
		return nil, fmt.Errorf("total partitions must be positive, got %d", cfg.TotalPartitions)
	}
	if len(cfg.Brokers) == 0 {
		return nil, fmt.Errorf("at least one broker is required")
	}
	if cfg.Topic == "" {
		return nil, fmt.Errorf("topic is required")
	}
	if cfg.Logger == nil {
		cfg.Logger = zap.NewNop()
	}

	// Calculate partitions for this shard
	partitions := GetPartitionsForShard(cfg.ShardID, cfg.ShardCount, cfg.TotalPartitions)
	if len(partitions) == 0 {
		return nil, fmt.Errorf("no partitions assigned to shard %d", cfg.ShardID)
	}

	// Set defaults
	if cfg.SessionTimeout == 0 {
		cfg.SessionTimeout = 30 * time.Second
	}
	if cfg.RebalanceTimeout == 0 {
		cfg.RebalanceTimeout = 60 * time.Second
	}

	// Build partition offsets map for direct partition assignment
	partitionOffsets := make(map[string]map[int32]kgo.Offset)
	topicOffsets := make(map[int32]kgo.Offset)
	for _, p := range partitions {
		topicOffsets[p] = kgo.NewOffset().AtCommitted()
	}
	partitionOffsets[cfg.Topic] = topicOffsets

	// Create Kafka client options
	opts := []kgo.Opt{
		kgo.SeedBrokers(cfg.Brokers...),
		// Use direct partition assignment instead of consumer group
		kgo.ConsumePartitions(partitionOffsets),
		// Still use consumer group for offset storage
		kgo.ConsumerGroup(cfg.ConsumerGroup),
		// Producer settings (if needed for publishing)
		kgo.ProducerBatchCompression(kgo.NoCompression()),
		kgo.ProducerLinger(10 * time.Millisecond),
		kgo.RequiredAcks(kgo.AllISRAcks()),
		kgo.RetryTimeout(10 * time.Second),
	}

	if cfg.DisableAutoCommit {
		opts = append(opts, kgo.DisableAutoCommit())
	}
	opts = append(opts, infra.KafkaSecurityOpts()...)

	client, err := kgo.NewClient(opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create Kafka client: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	sc := &ShardedConsumer{
		shardID:       cfg.ShardID,
		shardCount:    cfg.ShardCount,
		client:        client,
		partitions:    partitions,
		topic:         cfg.Topic,
		brokers:       cfg.Brokers,
		consumerGroup: cfg.ConsumerGroup,
		log:           cfg.Logger,
		ctx:           ctx,
		cancel:        cancel,
	}

	cfg.Logger.Info("Created sharded consumer",
		zap.Int("shard_id", cfg.ShardID),
		zap.Int("shard_count", cfg.ShardCount),
		zap.Int("total_partitions", cfg.TotalPartitions),
		zap.Int32s("assigned_partitions", partitions),
		zap.String("topic", cfg.Topic),
		zap.String("consumer_group", cfg.ConsumerGroup))

	return sc, nil
}

// SetMetrics sets the metrics instance for the consumer.
func (sc *ShardedConsumer) SetMetrics(metrics *ShardedConsumerMetrics) {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	sc.metrics = metrics
	if metrics != nil {
		metrics.AssignedPartitions.Set(float64(len(sc.partitions)))
	}
}

// Client returns the underlying Kafka client.
func (sc *ShardedConsumer) Client() *kgo.Client {
	return sc.client
}

// Partitions returns the partitions assigned to this consumer.
func (sc *ShardedConsumer) Partitions() []int32 {
	return sc.partitions
}

// ShardID returns the shard ID of this consumer.
func (sc *ShardedConsumer) ShardID() int {
	return sc.shardID
}

// Topic returns the topic this consumer is subscribed to.
func (sc *ShardedConsumer) Topic() string {
	return sc.topic
}

// PollFetches polls for new records from assigned partitions.
func (sc *ShardedConsumer) PollFetches(ctx context.Context) kgo.Fetches {
	fetches := sc.client.PollFetches(ctx)

	// Update metrics if available
	sc.mu.RLock()
	metrics := sc.metrics
	sc.mu.RUnlock()

	if metrics != nil {
		if err := fetches.Err(); err != nil {
			metrics.FetchErrors.Inc()
		}
	}

	return fetches
}

// CommitOffsets commits the current offsets for all assigned partitions.
func (sc *ShardedConsumer) CommitOffsets(ctx context.Context) error {
	err := sc.client.CommitUncommittedOffsets(ctx)

	sc.mu.RLock()
	metrics := sc.metrics
	sc.mu.RUnlock()

	if err != nil && metrics != nil {
		metrics.CommitErrors.Inc()
	}

	return err
}

// RecordProcessed records that a message was processed (for metrics).
func (sc *ShardedConsumer) RecordProcessed(partition int32, offset int64, processingTime time.Duration) {
	sc.mu.RLock()
	metrics := sc.metrics
	sc.mu.RUnlock()

	if metrics == nil {
		return
	}

	partStr := strconv.FormatInt(int64(partition), 10)
	metrics.MessagesProcessed.WithLabelValues(sc.topic, partStr).Inc()
	metrics.ProcessingLatency.WithLabelValues(sc.topic, partStr).Observe(processingTime.Seconds())
	metrics.CurrentOffset.WithLabelValues(sc.topic, partStr).Set(float64(offset))
}

// RecordError records a processing error (for metrics).
func (sc *ShardedConsumer) RecordError() {
	sc.mu.RLock()
	metrics := sc.metrics
	sc.mu.RUnlock()

	if metrics != nil {
		metrics.ProcessErrors.Inc()
	}
}

// UpdateLagMetrics updates consumer lag metrics by querying broker.
func (sc *ShardedConsumer) UpdateLagMetrics(ctx context.Context) error {
	sc.mu.RLock()
	metrics := sc.metrics
	sc.mu.RUnlock()

	if metrics == nil {
		return nil
	}

	// Create admin client to query offsets
	adminClient := kadm.NewClient(sc.client)

	// Get end offsets (high watermarks)
	endOffsets, err := adminClient.ListEndOffsets(ctx, sc.topic)
	if err != nil {
		return fmt.Errorf("failed to list end offsets: %w", err)
	}

	// Get committed offsets
	committedOffsets, err := adminClient.FetchOffsets(ctx, sc.consumerGroup)
	if err != nil {
		return fmt.Errorf("failed to fetch committed offsets: %w", err)
	}

	// Calculate and record lag for each assigned partition
	for _, p := range sc.partitions {
		partStr := strconv.FormatInt(int64(p), 10)

		// Get high watermark
		var hwm int64
		if topicOffsets, ok := endOffsets[sc.topic]; ok {
			if partOffset, ok := topicOffsets[p]; ok {
				hwm = partOffset.Offset
				metrics.HighWatermark.WithLabelValues(sc.topic, partStr).Set(float64(hwm))
			}
		}

		// Get committed offset
		var committed int64
		if topicCommitted, ok := committedOffsets.Lookup(sc.topic, p); ok {
			committed = topicCommitted.At
			metrics.CommittedOffset.WithLabelValues(sc.topic, partStr).Set(float64(committed))
		}

		// Calculate lag
		lag := hwm - committed
		if lag < 0 {
			lag = 0
		}
		metrics.ConsumerLag.WithLabelValues(sc.topic, partStr).Set(float64(lag))
	}

	return nil
}

// StartLagMonitor starts a background goroutine that periodically updates lag metrics.
func (sc *ShardedConsumer) StartLagMonitor(interval time.Duration) {
	infra.SafeGo(sc.log, "consumer-lag-monitor", func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-sc.ctx.Done():
				return
			case <-ticker.C:
				if err := sc.UpdateLagMetrics(sc.ctx); err != nil {
					sc.log.Warn("Failed to update lag metrics", zap.Error(err))
				}
			}
		}
	})
}

// Close closes the consumer and releases resources.
func (sc *ShardedConsumer) Close() {
	sc.cancel()
	sc.client.Close()
}

// GetShardIDFromPodName extracts shard ID from Kubernetes StatefulSet pod name.
// Pod names follow the pattern: {statefulset-name}-{ordinal}
// Examples:
//   - trading-engine-0 → 0
//   - trading-engine-1 → 1
//   - trading-engine-2 → 2
func GetShardIDFromPodName(podName string) (int, error) {
	if podName == "" {
		return 0, fmt.Errorf("pod name is empty")
	}

	// Match pattern: anything-{number}
	re := regexp.MustCompile(`-(\d+)$`)
	matches := re.FindStringSubmatch(podName)
	if len(matches) != 2 {
		return 0, fmt.Errorf("pod name %q does not match StatefulSet pattern", podName)
	}

	shardID, err := strconv.Atoi(matches[1])
	if err != nil {
		return 0, fmt.Errorf("failed to parse shard ID from pod name %q: %w", podName, err)
	}

	return shardID, nil
}

// GetShardIDFromEnv gets the shard ID from environment variables.
// It first tries POD_NAME (for Kubernetes StatefulSets), then falls back to SHARD_ID.
func GetShardIDFromEnv() (int, error) {
	// First try POD_NAME (Kubernetes StatefulSet pattern)
	podName := os.Getenv("POD_NAME")
	if podName != "" {
		shardID, err := GetShardIDFromPodName(podName)
		if err == nil {
			return shardID, nil
		}
		// Log warning but continue to fallback
	}

	// Fallback to explicit SHARD_ID
	shardIDStr := os.Getenv("SHARD_ID")
	if shardIDStr == "" {
		return 0, nil // Default to shard 0
	}

	shardID, err := strconv.Atoi(shardIDStr)
	if err != nil {
		return 0, fmt.Errorf("invalid SHARD_ID %q: %w", shardIDStr, err)
	}

	return shardID, nil
}

// BroadcastConsumer wraps a regular Kafka consumer for topics that all shards
// need to receive (like ticks). It uses a consumer group with a unique
// instance identifier to ensure all instances receive all messages.
type BroadcastConsumer struct {
	client     *kgo.Client
	topic      string
	instanceID string
	log        *zap.Logger
	metrics    *ShardedConsumerMetrics
	mu         sync.RWMutex
}

// BroadcastConsumerConfig holds configuration for a broadcast consumer.
type BroadcastConsumerConfig struct {
	Brokers    []string
	Topic      string
	InstanceID string // Unique identifier for this instance (e.g., pod name)
	Logger     *zap.Logger
}

// NewBroadcastConsumer creates a new consumer that receives all messages
// from a topic (broadcast pattern).
func NewBroadcastConsumer(cfg BroadcastConsumerConfig) (*BroadcastConsumer, error) {
	if len(cfg.Brokers) == 0 {
		return nil, fmt.Errorf("at least one broker is required")
	}
	if cfg.Topic == "" {
		return nil, fmt.Errorf("topic is required")
	}
	if cfg.InstanceID == "" {
		// Generate a unique ID if not provided
		cfg.InstanceID = fmt.Sprintf("broadcast-%d", time.Now().UnixNano())
	}
	if cfg.Logger == nil {
		cfg.Logger = zap.NewNop()
	}

	// Use instance-specific consumer group to receive all messages
	// Each instance has its own consumer group, so all instances receive all messages
	consumerGroup := fmt.Sprintf("trading-engine-ticks-%s", cfg.InstanceID)

	opts := []kgo.Opt{
		kgo.SeedBrokers(cfg.Brokers...),
		kgo.ConsumerGroup(consumerGroup),
		kgo.ConsumeTopics(cfg.Topic),
		kgo.DisableAutoCommit(),
	}
	opts = append(opts, infra.KafkaSecurityOpts()...)

	client, err := kgo.NewClient(opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create broadcast consumer: %w", err)
	}

	cfg.Logger.Info("Created broadcast consumer",
		zap.String("topic", cfg.Topic),
		zap.String("consumer_group", consumerGroup),
		zap.String("instance_id", cfg.InstanceID))

	return &BroadcastConsumer{
		client:     client,
		topic:      cfg.Topic,
		instanceID: cfg.InstanceID,
		log:        cfg.Logger,
	}, nil
}

// SetMetrics sets the metrics instance for the broadcast consumer.
func (bc *BroadcastConsumer) SetMetrics(metrics *ShardedConsumerMetrics) {
	bc.mu.Lock()
	defer bc.mu.Unlock()
	bc.metrics = metrics
}

// Client returns the underlying Kafka client.
func (bc *BroadcastConsumer) Client() *kgo.Client {
	return bc.client
}

// PollFetches polls for new records.
func (bc *BroadcastConsumer) PollFetches(ctx context.Context) kgo.Fetches {
	fetches := bc.client.PollFetches(ctx)

	bc.mu.RLock()
	metrics := bc.metrics
	bc.mu.RUnlock()

	if metrics != nil {
		if err := fetches.Err(); err != nil {
			metrics.FetchErrors.Inc()
		}
	}

	return fetches
}

// CommitOffsets commits the current offsets.
func (bc *BroadcastConsumer) CommitOffsets(ctx context.Context) error {
	err := bc.client.CommitUncommittedOffsets(ctx)

	bc.mu.RLock()
	metrics := bc.metrics
	bc.mu.RUnlock()

	if err != nil && metrics != nil {
		metrics.CommitErrors.Inc()
	}

	return err
}

// Close closes the broadcast consumer.
func (bc *BroadcastConsumer) Close() {
	bc.client.Close()
}

// PartitionConsumerStats holds statistics for a partition consumer.
type PartitionConsumerStats struct {
	ShardID            int     `json:"shard_id"`
	ShardCount         int     `json:"shard_count"`
	AssignedPartitions []int32 `json:"assigned_partitions"`
	Topic              string  `json:"topic"`
	ConsumerGroup      string  `json:"consumer_group"`
}

// GetStats returns statistics about the sharded consumer.
func (sc *ShardedConsumer) GetStats() PartitionConsumerStats {
	return PartitionConsumerStats{
		ShardID:            sc.shardID,
		ShardCount:         sc.shardCount,
		AssignedPartitions: sc.partitions,
		Topic:              sc.topic,
		ConsumerGroup:      sc.consumerGroup,
	}
}
