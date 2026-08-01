package redis

import (
	"testing"
)

func TestConfigFromEnv(t *testing.T) {
	tests := []struct {
		name     string
		env      map[string]string
		wantMode Mode
		wantDB   int
	}{
		{
			name:     "default standalone",
			env:      map[string]string{},
			wantMode: ModeStandalone,
			wantDB:   0,
		},
		{
			name: "explicit standalone",
			env: map[string]string{
				"REDIS_MODE": "standalone",
				"REDIS_ADDR": "redis:6379",
				"REDIS_DB":   "1",
			},
			wantMode: ModeStandalone,
			wantDB:   1,
		},
		{
			name: "sentinel mode from env",
			env: map[string]string{
				"REDIS_MODE":            "sentinel",
				"REDIS_SENTINEL_ADDRS":  "sentinel-1:26379,sentinel-2:26379",
				"REDIS_SENTINEL_MASTER": "mymaster",
			},
			wantMode: ModeSentinel,
			wantDB:   0,
		},
		{
			name: "auto-detect sentinel mode",
			env: map[string]string{
				"REDIS_SENTINEL_ADDRS":  "sentinel-1:26379,sentinel-2:26379",
				"REDIS_SENTINEL_MASTER": "mymaster",
			},
			wantMode: ModeSentinel,
			wantDB:   0,
		},
		{
			name: "cluster mode from env",
			env: map[string]string{
				"REDIS_MODE":          "cluster",
				"REDIS_CLUSTER_ADDRS": "node-1:6379,node-2:6379,node-3:6379",
			},
			wantMode: ModeCluster,
			wantDB:   0,
		},
		{
			name: "auto-detect cluster mode",
			env: map[string]string{
				"REDIS_CLUSTER_ADDRS": "node-1:6379,node-2:6379,node-3:6379",
			},
			wantMode: ModeCluster,
			wantDB:   0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			getenv := func(key string) string {
				return tt.env[key]
			}

			cfg := ConfigFromEnv(getenv)

			if cfg.Mode != tt.wantMode {
				t.Errorf("Mode = %v, want %v", cfg.Mode, tt.wantMode)
			}
			if cfg.DB != tt.wantDB {
				t.Errorf("DB = %v, want %v", cfg.DB, tt.wantDB)
			}
		})
	}
}

func TestNewClient_StandaloneValidation(t *testing.T) {
	cfg := Config{
		Mode: ModeStandalone,
		Addr: "localhost:6379",
	}

	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	defer client.Close()

	if client.Mode() != ModeStandalone {
		t.Errorf("Mode() = %v, want %v", client.Mode(), ModeStandalone)
	}
	if client.IsCluster() {
		t.Error("IsCluster() = true, want false")
	}
	if client.IsSentinel() {
		t.Error("IsSentinel() = true, want false")
	}
}

func TestNewClient_SentinelValidation(t *testing.T) {
	// Test missing sentinel addresses
	cfg := Config{
		Mode: ModeSentinel,
	}

	_, err := NewClient(cfg)
	if err == nil {
		t.Error("NewClient() with missing sentinel addrs should fail")
	}

	// Test missing master name
	cfg = Config{
		Mode:          ModeSentinel,
		SentinelAddrs: []string{"sentinel:26379"},
	}

	_, err = NewClient(cfg)
	if err == nil {
		t.Error("NewClient() with missing master name should fail")
	}

	// Test valid sentinel config (client creation should succeed, connection may fail)
	cfg = Config{
		Mode:           ModeSentinel,
		SentinelAddrs:  []string{"sentinel:26379"},
		SentinelMaster: "mymaster",
	}

	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	defer client.Close()

	if !client.IsSentinel() {
		t.Error("IsSentinel() = false, want true")
	}
}

func TestNewClient_ClusterValidation(t *testing.T) {
	// Test missing cluster addresses
	cfg := Config{
		Mode: ModeCluster,
	}

	_, err := NewClient(cfg)
	if err == nil {
		t.Error("NewClient() with missing cluster addrs should fail")
	}

	// Test valid cluster config (client creation should succeed, connection may fail)
	cfg = Config{
		Mode:         ModeCluster,
		ClusterAddrs: []string{"node-1:6379", "node-2:6379"},
	}

	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	defer client.Close()

	if !client.IsCluster() {
		t.Error("IsCluster() = false, want true")
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Mode != ModeStandalone {
		t.Errorf("Mode = %v, want %v", cfg.Mode, ModeStandalone)
	}
	if cfg.Addr != "localhost:6379" {
		t.Errorf("Addr = %v, want localhost:6379", cfg.Addr)
	}
	if cfg.DialTimeout == 0 {
		t.Error("DialTimeout should not be 0")
	}
	if cfg.ReadTimeout == 0 {
		t.Error("ReadTimeout should not be 0")
	}
	if cfg.WriteTimeout == 0 {
		t.Error("WriteTimeout should not be 0")
	}
}
