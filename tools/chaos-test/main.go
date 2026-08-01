// Package main provides a chaos engineering framework for validating system
// resilience under failure conditions. It supports various chaos scenarios
// including pod kills, network partitions, database failover, and more.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"sort"
	"strings"
	"syscall"
	"time"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// Config holds the CLI configuration
type Config struct {
	Scenario     string
	Namespace    string
	Kubeconfig   string
	ListOnly     bool
	DryRun       bool
	Timeout      time.Duration
	OutputFormat string
	WithLoad     bool
	LoadUsers    int
	LoadDuration time.Duration
	BaseURL      string
	Email        string
	Password     string
	ContestID    string
}

// ChaosScenario defines the interface for chaos scenarios
type ChaosScenario interface {
	Name() string
	Description() string
	Setup(ctx context.Context, clientset *kubernetes.Clientset, namespace string) error
	Run(ctx context.Context) error
	Verify(ctx context.Context) error
	Cleanup(ctx context.Context) error
	GetMetrics() *ScenarioMetrics
}

// TestResult holds the result of a chaos test
type TestResult struct {
	Scenario    string           `json:"scenario"`
	Description string           `json:"description"`
	StartTime   time.Time        `json:"start_time"`
	EndTime     time.Time        `json:"end_time"`
	Duration    time.Duration    `json:"duration"`
	Success     bool             `json:"success"`
	Error       string           `json:"error,omitempty"`
	Metrics     TestMetrics      `json:"metrics"`
	Phases      []PhaseResult    `json:"phases"`
}

// TestMetrics holds metrics collected during the test
type TestMetrics struct {
	ErrorsBeforeChaos   int           `json:"errors_before_chaos"`
	ErrorsDuringChaos   int           `json:"errors_during_chaos"`
	ErrorsAfterRecovery int           `json:"errors_after_recovery"`
	RecoveryTime        time.Duration `json:"recovery_time"`
	DataLoss            bool          `json:"data_loss"`
	P50Latency          time.Duration `json:"p50_latency"`
	P95Latency          time.Duration `json:"p95_latency"`
	P99Latency          time.Duration `json:"p99_latency"`
	RequestsTotal       int64         `json:"requests_total"`
	RequestsSuccessful  int64         `json:"requests_successful"`
	RequestsFailed      int64         `json:"requests_failed"`
}

// PhaseResult holds the result of a single test phase
type PhaseResult struct {
	Name      string        `json:"name"`
	StartTime time.Time     `json:"start_time"`
	EndTime   time.Time     `json:"end_time"`
	Duration  time.Duration `json:"duration"`
	Success   bool          `json:"success"`
	Error     string        `json:"error,omitempty"`
	Details   string        `json:"details,omitempty"`
}

// ScenarioMetrics holds metrics specific to a scenario
type ScenarioMetrics struct {
	PodsKilled         int           `json:"pods_killed,omitempty"`
	PodRecoveryTime    time.Duration `json:"pod_recovery_time,omitempty"`
	NetworkPolicyName  string        `json:"network_policy_name,omitempty"`
	PartitionDuration  time.Duration `json:"partition_duration,omitempty"`
	FailoverTime       time.Duration `json:"failover_time,omitempty"`
	DataIntegrityCheck bool          `json:"data_integrity_check,omitempty"`
}

func main() {
	cfg := parseFlags()

	if cfg.ListOnly {
		listScenarios()
		return
	}

	if cfg.Scenario == "" {
		fmt.Fprintln(os.Stderr, "Error: scenario is required. Use -list to see available scenarios.")
		os.Exit(1)
	}

	// Setup signal handling
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Println("\nReceived interrupt signal, cleaning up...")
		cancel()
	}()

	if err := run(ctx, cfg); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func parseFlags() *Config {
	cfg := &Config{}

	flag.StringVar(&cfg.Scenario, "scenario", "", "Chaos scenario to run (use -list to see available)")
	flag.StringVar(&cfg.Namespace, "namespace", "tragge", "Kubernetes namespace")
	flag.StringVar(&cfg.Kubeconfig, "kubeconfig", "", "Path to kubeconfig (uses in-cluster config if empty)")
	flag.BoolVar(&cfg.ListOnly, "list", false, "List available scenarios")
	flag.BoolVar(&cfg.DryRun, "dry-run", false, "Print what would be done without executing")
	flag.DurationVar(&cfg.Timeout, "timeout", 10*time.Minute, "Overall test timeout")
	flag.StringVar(&cfg.OutputFormat, "output", "text", "Output format: text, json")
	flag.BoolVar(&cfg.WithLoad, "with-load", false, "Run scenario with background load")
	flag.IntVar(&cfg.LoadUsers, "load-users", 50, "Number of virtual users for load test")
	flag.DurationVar(&cfg.LoadDuration, "load-duration", 5*time.Minute, "Duration of load test")
	flag.StringVar(&cfg.BaseURL, "base-url", "http://localhost:8080", "Base URL for API calls")
	flag.StringVar(&cfg.Email, "email", "", "Email for authentication (required for load tests)")
	flag.StringVar(&cfg.Password, "password", "", "Password for authentication (required for load tests)")
	flag.StringVar(&cfg.ContestID, "contest-id", "", "Contest ID for trading tests")

	flag.Parse()
	return cfg
}

func listScenarios() {
	fmt.Println("Available Chaos Scenarios:")
	fmt.Println()

	// Get sorted scenario names
	names := make([]string, 0, len(scenarios))
	for name := range scenarios {
		names = append(names, name)
	}
	sort.Strings(names)

	// Find max name length for alignment
	maxLen := 0
	for _, name := range names {
		if len(name) > maxLen {
			maxLen = len(name)
		}
	}

	for _, name := range names {
		s := scenarios[name]
		fmt.Printf("  %-*s  %s\n", maxLen, name, s.Description())
	}

	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  chaos-test -scenario=pod-kill -namespace=tragge")
	fmt.Println("  chaos-test -scenario=pod-kill-trading -with-load -email=test@example.com -password=pass123")
	fmt.Println("  chaos-test -scenario=all -output=json > results.json")
}

// scenarios registry
var scenarios = map[string]ChaosScenario{
	// Basic pod kill scenarios
	"pod-kill":         &PodKillScenario{},
	"pod-kill-trading": &TradingEnginePodKillScenario{},
	"pod-kill-bff":     &BFFPodKillScenario{},

	// Network scenarios
	"network-partition":         &NetworkPartitionScenario{},
	"network-partition-iptables": &NetworkPartitionIPTablesScenario{},

	// Database scenarios
	"db-failover":      &DBFailoverScenario{},
	"database-failure": &DatabaseFailureScenario{},

	// Redis scenarios
	"redis-failover": &RedisFailoverScenario{},
	"redis-failure":  &RedisFailureScenario{},

	// Kafka scenarios
	"kafka-partition":      &KafkaPartitionScenario{},
	"kafka-broker-failure": &KafkaBrokerFailureScenario{},

	// Resource pressure scenarios
	"high-cpu":         &HighCPUScenario{},
	"memory-pressure":  &MemoryPressureScenario{},

	// Other scenarios
	"dns-failure":      &DNSFailureScenario{},
	"cascade-failure":  &CascadeFailureScenario{},
}

func run(ctx context.Context, cfg *Config) error {
	// Build Kubernetes client
	clientset, err := buildKubeClient(cfg.Kubeconfig)
	if err != nil {
		return fmt.Errorf("failed to create Kubernetes client: %w", err)
	}

	// Apply timeout
	ctx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()

	var results []TestResult

	if cfg.Scenario == "all" {
		// Run all scenarios
		names := make([]string, 0, len(scenarios))
		for name := range scenarios {
			names = append(names, name)
		}
		sort.Strings(names)

		for _, name := range names {
			fmt.Printf("\n=== Running scenario: %s ===\n", name)
			result := runScenario(ctx, scenarios[name], clientset, cfg)
			results = append(results, result)

			if !result.Success {
				fmt.Printf("Scenario %s FAILED: %s\n", name, result.Error)
			} else {
				fmt.Printf("Scenario %s PASSED (recovery: %s)\n", name, result.Metrics.RecoveryTime)
			}
		}
	} else {
		// Run specific scenario
		s, ok := scenarios[cfg.Scenario]
		if !ok {
			return fmt.Errorf("unknown scenario: %s (use -list to see available)", cfg.Scenario)
		}

		if cfg.DryRun {
			fmt.Printf("Would run scenario: %s\n", s.Name())
			fmt.Printf("Description: %s\n", s.Description())
			fmt.Printf("Namespace: %s\n", cfg.Namespace)
			fmt.Printf("Timeout: %s\n", cfg.Timeout)
			if cfg.WithLoad {
				fmt.Printf("With load: %d users for %s\n", cfg.LoadUsers, cfg.LoadDuration)
			}
			return nil
		}

		result := runScenario(ctx, s, clientset, cfg)
		results = append(results, result)
	}

	// Output results
	return outputResults(results, cfg.OutputFormat)
}

func buildKubeClient(kubeconfig string) (*kubernetes.Clientset, error) {
	var config *rest.Config
	var err error

	if kubeconfig != "" {
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
	} else {
		// Try in-cluster config first
		config, err = rest.InClusterConfig()
		if err != nil {
			// Fall back to default kubeconfig location
			home, _ := os.UserHomeDir()
			defaultKubeconfig := home + "/.kube/config"
			config, err = clientcmd.BuildConfigFromFlags("", defaultKubeconfig)
		}
	}

	if err != nil {
		return nil, fmt.Errorf("failed to build config: %w", err)
	}

	return kubernetes.NewForConfig(config)
}

func runScenario(ctx context.Context, s ChaosScenario, clientset *kubernetes.Clientset, cfg *Config) TestResult {
	result := TestResult{
		Scenario:    s.Name(),
		Description: s.Description(),
		StartTime:   time.Now(),
		Phases:      make([]PhaseResult, 0),
	}

	defer func() {
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(result.StartTime)
	}()

	// Phase 1: Setup
	phase := PhaseResult{Name: "setup", StartTime: time.Now()}
	fmt.Printf("[%s] Setting up scenario...\n", s.Name())

	if err := s.Setup(ctx, clientset, cfg.Namespace); err != nil {
		phase.EndTime = time.Now()
		phase.Duration = phase.EndTime.Sub(phase.StartTime)
		phase.Success = false
		phase.Error = err.Error()
		result.Phases = append(result.Phases, phase)
		result.Success = false
		result.Error = fmt.Sprintf("setup failed: %v", err)
		return result
	}

	phase.EndTime = time.Now()
	phase.Duration = phase.EndTime.Sub(phase.StartTime)
	phase.Success = true
	result.Phases = append(result.Phases, phase)

	// Ensure cleanup runs
	defer func() {
		cleanupPhase := PhaseResult{Name: "cleanup", StartTime: time.Now()}
		fmt.Printf("[%s] Cleaning up...\n", s.Name())

		cleanupCtx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()

		if err := s.Cleanup(cleanupCtx); err != nil {
			cleanupPhase.Error = err.Error()
			cleanupPhase.Success = false
			fmt.Printf("[%s] Cleanup warning: %v\n", s.Name(), err)
		} else {
			cleanupPhase.Success = true
		}

		cleanupPhase.EndTime = time.Now()
		cleanupPhase.Duration = cleanupPhase.EndTime.Sub(cleanupPhase.StartTime)
		result.Phases = append(result.Phases, cleanupPhase)
	}()

	// Phase 2: Run chaos
	chaosPhase := PhaseResult{Name: "chaos", StartTime: time.Now()}
	fmt.Printf("[%s] Injecting chaos...\n", s.Name())

	if err := s.Run(ctx); err != nil {
		chaosPhase.EndTime = time.Now()
		chaosPhase.Duration = chaosPhase.EndTime.Sub(chaosPhase.StartTime)
		chaosPhase.Success = false
		chaosPhase.Error = err.Error()
		result.Phases = append(result.Phases, chaosPhase)
		result.Success = false
		result.Error = fmt.Sprintf("chaos injection failed: %v", err)
		return result
	}

	chaosPhase.EndTime = time.Now()
	chaosPhase.Duration = chaosPhase.EndTime.Sub(chaosPhase.StartTime)
	chaosPhase.Success = true
	result.Phases = append(result.Phases, chaosPhase)

	// Phase 3: Verify recovery
	verifyPhase := PhaseResult{Name: "verify", StartTime: time.Now()}
	fmt.Printf("[%s] Verifying recovery...\n", s.Name())

	recoveryStart := time.Now()
	if err := s.Verify(ctx); err != nil {
		verifyPhase.EndTime = time.Now()
		verifyPhase.Duration = verifyPhase.EndTime.Sub(verifyPhase.StartTime)
		verifyPhase.Success = false
		verifyPhase.Error = err.Error()
		result.Phases = append(result.Phases, verifyPhase)
		result.Success = false
		result.Error = fmt.Sprintf("verification failed: %v", err)
		return result
	}

	result.Metrics.RecoveryTime = time.Since(recoveryStart)
	verifyPhase.EndTime = time.Now()
	verifyPhase.Duration = verifyPhase.EndTime.Sub(verifyPhase.StartTime)
	verifyPhase.Success = true
	verifyPhase.Details = fmt.Sprintf("Recovery time: %s", result.Metrics.RecoveryTime)
	result.Phases = append(result.Phases, verifyPhase)

	// Copy scenario-specific metrics
	if metrics := s.GetMetrics(); metrics != nil {
		if metrics.PodRecoveryTime > 0 {
			result.Metrics.RecoveryTime = metrics.PodRecoveryTime
		}
		result.Metrics.DataLoss = !metrics.DataIntegrityCheck
	}

	result.Success = true
	fmt.Printf("[%s] Scenario completed successfully\n", s.Name())

	return result
}

func outputResults(results []TestResult, format string) error {
	switch format {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(results)

	case "text":
		return outputTextResults(results)

	default:
		return fmt.Errorf("unknown output format: %s", format)
	}
}

func outputTextResults(results []TestResult) error {
	fmt.Println()
	fmt.Println(strings.Repeat("=", 80))
	fmt.Println("                        CHAOS TEST RESULTS")
	fmt.Println(strings.Repeat("=", 80))
	fmt.Println()

	passed := 0
	failed := 0

	for _, r := range results {
		status := "PASSED"
		if !r.Success {
			status = "FAILED"
			failed++
		} else {
			passed++
		}

		fmt.Printf("Scenario: %s\n", r.Scenario)
		fmt.Printf("Status:   %s\n", status)
		fmt.Printf("Duration: %s\n", r.Duration.Round(time.Millisecond))

		if r.Success {
			fmt.Printf("Recovery: %s\n", r.Metrics.RecoveryTime.Round(time.Millisecond))
		} else {
			fmt.Printf("Error:    %s\n", r.Error)
		}

		fmt.Println()
		fmt.Println("Phases:")
		for _, p := range r.Phases {
			pStatus := "OK"
			if !p.Success {
				pStatus = "FAIL"
			}
			fmt.Printf("  - %-10s [%s] %s\n", p.Name, pStatus, p.Duration.Round(time.Millisecond))
			if p.Details != "" {
				fmt.Printf("               %s\n", p.Details)
			}
			if p.Error != "" {
				fmt.Printf("               Error: %s\n", p.Error)
			}
		}

		fmt.Println()
		fmt.Println(strings.Repeat("-", 80))
		fmt.Println()
	}

	fmt.Printf("Summary: %d passed, %d failed out of %d scenarios\n", passed, failed, len(results))

	if failed > 0 {
		return fmt.Errorf("%d scenarios failed", failed)
	}

	return nil
}
