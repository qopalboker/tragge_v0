package server

import (
	"testing"
)

func TestGetPartitionsForShard(t *testing.T) {
	tests := []struct {
		name            string
		shardID         int
		shardCount      int
		totalPartitions int
		want            []int32
	}{
		{
			name:            "4 shards, 16 partitions, shard 0",
			shardID:         0,
			shardCount:      4,
			totalPartitions: 16,
			want:            []int32{0, 4, 8, 12},
		},
		{
			name:            "4 shards, 16 partitions, shard 1",
			shardID:         1,
			shardCount:      4,
			totalPartitions: 16,
			want:            []int32{1, 5, 9, 13},
		},
		{
			name:            "4 shards, 16 partitions, shard 2",
			shardID:         2,
			shardCount:      4,
			totalPartitions: 16,
			want:            []int32{2, 6, 10, 14},
		},
		{
			name:            "4 shards, 16 partitions, shard 3",
			shardID:         3,
			shardCount:      4,
			totalPartitions: 16,
			want:            []int32{3, 7, 11, 15},
		},
		{
			name:            "2 shards, 8 partitions, shard 0",
			shardID:         0,
			shardCount:      2,
			totalPartitions: 8,
			want:            []int32{0, 2, 4, 6},
		},
		{
			name:            "2 shards, 8 partitions, shard 1",
			shardID:         1,
			shardCount:      2,
			totalPartitions: 8,
			want:            []int32{1, 3, 5, 7},
		},
		{
			name:            "1 shard, 4 partitions, shard 0",
			shardID:         0,
			shardCount:      1,
			totalPartitions: 4,
			want:            []int32{0, 1, 2, 3},
		},
		{
			name:            "3 shards, 9 partitions, shard 0",
			shardID:         0,
			shardCount:      3,
			totalPartitions: 9,
			want:            []int32{0, 3, 6},
		},
		{
			name:            "3 shards, 9 partitions, shard 1",
			shardID:         1,
			shardCount:      3,
			totalPartitions: 9,
			want:            []int32{1, 4, 7},
		},
		{
			name:            "3 shards, 9 partitions, shard 2",
			shardID:         2,
			shardCount:      3,
			totalPartitions: 9,
			want:            []int32{2, 5, 8},
		},
		{
			name:            "uneven: 3 shards, 10 partitions, shard 0",
			shardID:         0,
			shardCount:      3,
			totalPartitions: 10,
			want:            []int32{0, 3, 6, 9},
		},
		{
			name:            "uneven: 3 shards, 10 partitions, shard 1",
			shardID:         1,
			shardCount:      3,
			totalPartitions: 10,
			want:            []int32{1, 4, 7},
		},
		{
			name:            "uneven: 3 shards, 10 partitions, shard 2",
			shardID:         2,
			shardCount:      3,
			totalPartitions: 10,
			want:            []int32{2, 5, 8},
		},
		// Edge cases
		{
			name:            "invalid: negative shard ID",
			shardID:         -1,
			shardCount:      4,
			totalPartitions: 16,
			want:            nil,
		},
		{
			name:            "invalid: shard ID >= shard count",
			shardID:         4,
			shardCount:      4,
			totalPartitions: 16,
			want:            nil,
		},
		{
			name:            "invalid: zero shard count",
			shardID:         0,
			shardCount:      0,
			totalPartitions: 16,
			want:            nil,
		},
		{
			name:            "invalid: zero partitions",
			shardID:         0,
			shardCount:      4,
			totalPartitions: 0,
			want:            nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetPartitionsForShard(tt.shardID, tt.shardCount, tt.totalPartitions)

			if len(got) != len(tt.want) {
				t.Errorf("GetPartitionsForShard() returned %d partitions, want %d", len(got), len(tt.want))
				return
			}

			for i, p := range got {
				if p != tt.want[i] {
					t.Errorf("GetPartitionsForShard()[%d] = %d, want %d", i, p, tt.want[i])
				}
			}
		})
	}
}

func TestGetPartitionsForShard_AllPartitionsCovered(t *testing.T) {
	// Verify that all partitions are covered exactly once across all shards
	testCases := []struct {
		shardCount      int
		totalPartitions int
	}{
		{4, 16},
		{2, 8},
		{3, 9},
		{3, 10},
		{5, 20},
		{1, 4},
	}

	for _, tc := range testCases {
		t.Run("", func(t *testing.T) {
			seen := make(map[int32]int)

			for shardID := 0; shardID < tc.shardCount; shardID++ {
				partitions := GetPartitionsForShard(shardID, tc.shardCount, tc.totalPartitions)
				for _, p := range partitions {
					seen[p]++
				}
			}

			// Verify all partitions are covered exactly once
			for p := int32(0); p < int32(tc.totalPartitions); p++ {
				if seen[p] != 1 {
					t.Errorf("Partition %d was assigned %d times (expected 1) for shardCount=%d, totalPartitions=%d",
						p, seen[p], tc.shardCount, tc.totalPartitions)
				}
			}
		})
	}
}

func TestGetShardIDFromPodName(t *testing.T) {
	tests := []struct {
		name    string
		podName string
		want    int
		wantErr bool
	}{
		{
			name:    "trading-engine-0",
			podName: "trading-engine-0",
			want:    0,
			wantErr: false,
		},
		{
			name:    "trading-engine-1",
			podName: "trading-engine-1",
			want:    1,
			wantErr: false,
		},
		{
			name:    "trading-engine-10",
			podName: "trading-engine-10",
			want:    10,
			wantErr: false,
		},
		{
			name:    "trading-engine-99",
			podName: "trading-engine-99",
			want:    99,
			wantErr: false,
		},
		{
			name:    "custom-service-5",
			podName: "custom-service-5",
			want:    5,
			wantErr: false,
		},
		{
			name:    "my-app-with-dashes-3",
			podName: "my-app-with-dashes-3",
			want:    3,
			wantErr: false,
		},
		{
			name:    "empty string",
			podName: "",
			want:    0,
			wantErr: true,
		},
		{
			name:    "no ordinal suffix",
			podName: "trading-engine",
			want:    0,
			wantErr: true,
		},
		{
			name:    "invalid ordinal",
			podName: "trading-engine-abc",
			want:    0,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GetShardIDFromPodName(tt.podName)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetShardIDFromPodName() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("GetShardIDFromPodName() = %v, want %v", got, tt.want)
			}
		})
	}
}

