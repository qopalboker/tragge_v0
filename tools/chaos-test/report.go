package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"math"
	"os"
	"sort"
	"strings"
	"time"
)

// ReportGenerator generates reports from test results
type ReportGenerator struct {
	results   []TestResult
	timestamp time.Time
}

// NewReportGenerator creates a new report generator
func NewReportGenerator(results []TestResult) *ReportGenerator {
	return &ReportGenerator{
		results:   results,
		timestamp: time.Now(),
	}
}

// GenerateHTML generates an HTML report
func (rg *ReportGenerator) GenerateHTML(outputPath string) error {
	tmpl := template.Must(template.New("report").Funcs(template.FuncMap{
		"statusClass": func(success bool) string {
			if success {
				return "success"
			}
			return "failure"
		},
		"statusIcon": func(success bool) string {
			if success {
				return "&#10003;" // checkmark
			}
			return "&#10007;" // X mark
		},
		"formatDuration": func(d time.Duration) string {
			return d.Round(time.Millisecond).String()
		},
		"percentage": func(a, b int64) string {
			if b == 0 {
				return "N/A"
			}
			return fmt.Sprintf("%.1f%%", float64(a)/float64(b)*100)
		},
	}).Parse(htmlTemplate))

	f, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("create report file: %w", err)
	}
	defer f.Close()

	data := struct {
		Title     string
		Timestamp string
		Results   []TestResult
		Summary   ReportSummary
	}{
		Title:     "Chaos Test Report",
		Timestamp: rg.timestamp.Format(time.RFC3339),
		Results:   rg.results,
		Summary:   rg.calculateSummary(),
	}

	return tmpl.Execute(f, data)
}

// ReportSummary holds summary statistics
type ReportSummary struct {
	TotalScenarios   int
	PassedScenarios  int
	FailedScenarios  int
	TotalDuration    time.Duration
	AvgRecoveryTime  time.Duration
	MaxRecoveryTime  time.Duration
	MinRecoveryTime  time.Duration
	DataLossDetected bool

	// Categorized results
	DatabaseScenarios  CategorySummary
	RedisScenarios     CategorySummary
	KafkaScenarios     CategorySummary
	NetworkScenarios   CategorySummary
	OtherScenarios     CategorySummary

	// Data integrity
	IntegrityIssues []IntegrityIssue

	// Recovery time analysis
	RecoveryAnalysis RecoveryTimeAnalysis
}

// CategorySummary holds summary for a category of scenarios
type CategorySummary struct {
	Total   int
	Passed  int
	Failed  int
	Results []ScenarioResult
}

// ScenarioResult holds a single scenario result for reporting
type ScenarioResult struct {
	Name         string
	Passed       bool
	RecoveryTime time.Duration
	Error        string
}

// IntegrityIssue represents a data integrity issue found during testing
type IntegrityIssue struct {
	Scenario    string
	Description string
	Severity    string // critical, warning, info
	Timestamp   time.Time
}

// RecoveryTimeAnalysis provides statistical analysis of recovery times
type RecoveryTimeAnalysis struct {
	P50        time.Duration
	P90        time.Duration
	P99        time.Duration
	Median     time.Duration
	Mean       time.Duration
	StdDev     time.Duration
	Outliers   []string // Scenarios with unusually long recovery times
}

func (rg *ReportGenerator) calculateSummary() ReportSummary {
	summary := ReportSummary{
		IntegrityIssues:    make([]IntegrityIssue, 0),
		DatabaseScenarios:  CategorySummary{Results: make([]ScenarioResult, 0)},
		RedisScenarios:     CategorySummary{Results: make([]ScenarioResult, 0)},
		KafkaScenarios:     CategorySummary{Results: make([]ScenarioResult, 0)},
		NetworkScenarios:   CategorySummary{Results: make([]ScenarioResult, 0)},
		OtherScenarios:     CategorySummary{Results: make([]ScenarioResult, 0)},
	}

	var totalRecovery time.Duration
	recoveryTimes := make([]time.Duration, 0)

	for _, r := range rg.results {
		summary.TotalScenarios++
		if r.Success {
			summary.PassedScenarios++
		} else {
			summary.FailedScenarios++
		}
		summary.TotalDuration += r.Duration

		// Track recovery times
		if r.Metrics.RecoveryTime > 0 {
			totalRecovery += r.Metrics.RecoveryTime
			recoveryTimes = append(recoveryTimes, r.Metrics.RecoveryTime)
			if r.Metrics.RecoveryTime > summary.MaxRecoveryTime {
				summary.MaxRecoveryTime = r.Metrics.RecoveryTime
			}
			if summary.MinRecoveryTime == 0 || r.Metrics.RecoveryTime < summary.MinRecoveryTime {
				summary.MinRecoveryTime = r.Metrics.RecoveryTime
			}
		}

		// Track data integrity issues
		if r.Metrics.DataLoss {
			summary.DataLossDetected = true
			summary.IntegrityIssues = append(summary.IntegrityIssues, IntegrityIssue{
				Scenario:    r.Scenario,
				Description: "Data loss detected during chaos test",
				Severity:    "critical",
				Timestamp:   r.EndTime,
			})
		}

		// Categorize results
		scenarioResult := ScenarioResult{
			Name:         r.Scenario,
			Passed:       r.Success,
			RecoveryTime: r.Metrics.RecoveryTime,
			Error:        r.Error,
		}

		category := categorizeScenario(r.Scenario)
		switch category {
		case "database":
			summary.DatabaseScenarios.Total++
			if r.Success {
				summary.DatabaseScenarios.Passed++
			} else {
				summary.DatabaseScenarios.Failed++
			}
			summary.DatabaseScenarios.Results = append(summary.DatabaseScenarios.Results, scenarioResult)
		case "redis":
			summary.RedisScenarios.Total++
			if r.Success {
				summary.RedisScenarios.Passed++
			} else {
				summary.RedisScenarios.Failed++
			}
			summary.RedisScenarios.Results = append(summary.RedisScenarios.Results, scenarioResult)
		case "kafka":
			summary.KafkaScenarios.Total++
			if r.Success {
				summary.KafkaScenarios.Passed++
			} else {
				summary.KafkaScenarios.Failed++
			}
			summary.KafkaScenarios.Results = append(summary.KafkaScenarios.Results, scenarioResult)
		case "network":
			summary.NetworkScenarios.Total++
			if r.Success {
				summary.NetworkScenarios.Passed++
			} else {
				summary.NetworkScenarios.Failed++
			}
			summary.NetworkScenarios.Results = append(summary.NetworkScenarios.Results, scenarioResult)
		default:
			summary.OtherScenarios.Total++
			if r.Success {
				summary.OtherScenarios.Passed++
			} else {
				summary.OtherScenarios.Failed++
			}
			summary.OtherScenarios.Results = append(summary.OtherScenarios.Results, scenarioResult)
		}
	}

	// Calculate average recovery time
	if len(recoveryTimes) > 0 {
		summary.AvgRecoveryTime = totalRecovery / time.Duration(len(recoveryTimes))
		summary.RecoveryAnalysis = calculateRecoveryAnalysis(recoveryTimes, rg.results)
	}

	return summary
}

// categorizeScenario determines which category a scenario belongs to
func categorizeScenario(name string) string {
	switch {
	case strings.Contains(name, "database") || strings.Contains(name, "db-") || strings.Contains(name, "postgres"):
		return "database"
	case strings.Contains(name, "redis"):
		return "redis"
	case strings.Contains(name, "kafka") || strings.Contains(name, "redpanda"):
		return "kafka"
	case strings.Contains(name, "network") || strings.Contains(name, "partition") || strings.Contains(name, "dns"):
		return "network"
	default:
		return "other"
	}
}

// calculateRecoveryAnalysis computes statistical analysis of recovery times
func calculateRecoveryAnalysis(times []time.Duration, results []TestResult) RecoveryTimeAnalysis {
	if len(times) == 0 {
		return RecoveryTimeAnalysis{}
	}

	// Sort times for percentile calculation
	sorted := make([]time.Duration, len(times))
	copy(sorted, times)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i] < sorted[j]
	})

	n := len(sorted)
	analysis := RecoveryTimeAnalysis{
		Median: sorted[n/2],
		P50:    sorted[int(float64(n)*0.50)],
		P90:    sorted[int(float64(n)*0.90)],
		P99:    sorted[int(float64(n)*0.99)],
	}

	// Calculate mean
	var sum time.Duration
	for _, t := range sorted {
		sum += t
	}
	analysis.Mean = sum / time.Duration(n)

	// Calculate standard deviation
	var variance float64
	meanFloat := float64(analysis.Mean)
	for _, t := range sorted {
		diff := float64(t) - meanFloat
		variance += diff * diff
	}
	variance /= float64(n)
	analysis.StdDev = time.Duration(math.Sqrt(variance))

	// Find outliers (recovery times > 2 standard deviations above mean)
	threshold := analysis.Mean + 2*analysis.StdDev
	for _, r := range results {
		if r.Metrics.RecoveryTime > threshold {
			analysis.Outliers = append(analysis.Outliers, r.Scenario)
		}
	}

	return analysis
}

// writeCategorySummary generates a markdown table for a category of scenarios
func writeCategorySummary(cat *CategorySummary) string {
	var sb strings.Builder

	passRate := float64(cat.Passed) / float64(cat.Total) * 100
	status := "✅"
	if cat.Failed > 0 {
		if passRate < 50 {
			status = "❌"
		} else {
			status = "⚠️"
		}
	}

	sb.WriteString(fmt.Sprintf("**Status:** %s %d/%d passed (%.1f%%)\n\n",
		status, cat.Passed, cat.Total, passRate))

	sb.WriteString("| Scenario | Status | Recovery Time | Error |\n")
	sb.WriteString("|----------|--------|---------------|-------|\n")

	for _, r := range cat.Results {
		status := "PASS"
		if !r.Passed {
			status = "**FAIL**"
		}
		errMsg := "-"
		if r.Error != "" {
			errMsg = truncate(r.Error, 40)
		}
		sb.WriteString(fmt.Sprintf("| %s | %s | %s | %s |\n",
			r.Name, status, r.RecoveryTime.Round(time.Millisecond), errMsg))
	}
	sb.WriteString("\n")

	return sb.String()
}

// GenerateJSON generates a JSON report
func (rg *ReportGenerator) GenerateJSON(outputPath string) error {
	f, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("create JSON report: %w", err)
	}
	defer f.Close()

	report := struct {
		Timestamp string        `json:"timestamp"`
		Summary   ReportSummary `json:"summary"`
		Results   []TestResult  `json:"results"`
	}{
		Timestamp: rg.timestamp.Format(time.RFC3339),
		Summary:   rg.calculateSummary(),
		Results:   rg.results,
	}

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(report)
}

// GenerateMarkdown generates a Markdown report
func (rg *ReportGenerator) GenerateMarkdown(outputPath string) error {
	f, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("create markdown report: %w", err)
	}
	defer f.Close()

	var sb strings.Builder

	summary := rg.calculateSummary()

	sb.WriteString("# Chaos Test Report\n\n")
	sb.WriteString(fmt.Sprintf("**Generated:** %s\n\n", rg.timestamp.Format(time.RFC3339)))

	// Overall summary section
	sb.WriteString("## Executive Summary\n\n")

	// Pass/Fail status
	passRate := float64(summary.PassedScenarios) / float64(summary.TotalScenarios) * 100
	statusEmoji := "🟢"
	if summary.FailedScenarios > 0 {
		if passRate < 50 {
			statusEmoji = "🔴"
		} else {
			statusEmoji = "🟡"
		}
	}
	if summary.DataLossDetected {
		statusEmoji = "🔴"
	}

	sb.WriteString(fmt.Sprintf("**Overall Status:** %s %.1f%% Pass Rate (%d/%d)\n\n",
		statusEmoji, passRate, summary.PassedScenarios, summary.TotalScenarios))

	sb.WriteString("| Metric | Value |\n")
	sb.WriteString("|--------|-------|\n")
	sb.WriteString(fmt.Sprintf("| Total Scenarios | %d |\n", summary.TotalScenarios))
	sb.WriteString(fmt.Sprintf("| Passed | %d |\n", summary.PassedScenarios))
	sb.WriteString(fmt.Sprintf("| Failed | %d |\n", summary.FailedScenarios))
	sb.WriteString(fmt.Sprintf("| Total Duration | %s |\n", summary.TotalDuration.Round(time.Second)))
	sb.WriteString(fmt.Sprintf("| Avg Recovery Time | %s |\n", summary.AvgRecoveryTime.Round(time.Millisecond)))
	sb.WriteString(fmt.Sprintf("| Min Recovery Time | %s |\n", summary.MinRecoveryTime.Round(time.Millisecond)))
	sb.WriteString(fmt.Sprintf("| Max Recovery Time | %s |\n", summary.MaxRecoveryTime.Round(time.Millisecond)))

	if summary.DataLossDetected {
		sb.WriteString("| Data Loss | **DETECTED** ⚠️ |\n")
	} else {
		sb.WriteString("| Data Loss | None ✅ |\n")
	}
	sb.WriteString("\n")

	// Recovery Time Analysis
	sb.WriteString("## Recovery Time Analysis\n\n")
	sb.WriteString("| Percentile | Time |\n")
	sb.WriteString("|------------|------|\n")
	sb.WriteString(fmt.Sprintf("| P50 (Median) | %s |\n", summary.RecoveryAnalysis.P50.Round(time.Millisecond)))
	sb.WriteString(fmt.Sprintf("| P90 | %s |\n", summary.RecoveryAnalysis.P90.Round(time.Millisecond)))
	sb.WriteString(fmt.Sprintf("| P99 | %s |\n", summary.RecoveryAnalysis.P99.Round(time.Millisecond)))
	sb.WriteString(fmt.Sprintf("| Mean | %s |\n", summary.RecoveryAnalysis.Mean.Round(time.Millisecond)))
	sb.WriteString(fmt.Sprintf("| Std Dev | %s |\n", summary.RecoveryAnalysis.StdDev.Round(time.Millisecond)))
	sb.WriteString("\n")

	if len(summary.RecoveryAnalysis.Outliers) > 0 {
		sb.WriteString("**Recovery Time Outliers:**\n")
		for _, outlier := range summary.RecoveryAnalysis.Outliers {
			sb.WriteString(fmt.Sprintf("- %s\n", outlier))
		}
		sb.WriteString("\n")
	}

	// Category breakdown
	sb.WriteString("## Results by Category\n\n")

	// Database scenarios
	if summary.DatabaseScenarios.Total > 0 {
		sb.WriteString("### Database Scenarios\n\n")
		sb.WriteString(writeCategorySummary(&summary.DatabaseScenarios))
	}

	// Redis scenarios
	if summary.RedisScenarios.Total > 0 {
		sb.WriteString("### Redis Scenarios\n\n")
		sb.WriteString(writeCategorySummary(&summary.RedisScenarios))
	}

	// Kafka scenarios
	if summary.KafkaScenarios.Total > 0 {
		sb.WriteString("### Kafka Scenarios\n\n")
		sb.WriteString(writeCategorySummary(&summary.KafkaScenarios))
	}

	// Network scenarios
	if summary.NetworkScenarios.Total > 0 {
		sb.WriteString("### Network Scenarios\n\n")
		sb.WriteString(writeCategorySummary(&summary.NetworkScenarios))
	}

	// Other scenarios
	if summary.OtherScenarios.Total > 0 {
		sb.WriteString("### Other Scenarios\n\n")
		sb.WriteString(writeCategorySummary(&summary.OtherScenarios))
	}

	// Data Integrity Issues
	if len(summary.IntegrityIssues) > 0 {
		sb.WriteString("## ⚠️ Data Integrity Issues\n\n")
		sb.WriteString("| Scenario | Description | Severity | Time |\n")
		sb.WriteString("|----------|-------------|----------|------|\n")
		for _, issue := range summary.IntegrityIssues {
			severity := issue.Severity
			if severity == "critical" {
				severity = "**CRITICAL**"
			}
			sb.WriteString(fmt.Sprintf("| %s | %s | %s | %s |\n",
				issue.Scenario, issue.Description, severity, issue.Timestamp.Format(time.RFC3339)))
		}
		sb.WriteString("\n")
	}

	// Results table
	sb.WriteString("## Results\n\n")
	sb.WriteString("| Scenario | Status | Duration | Recovery Time | Error |\n")
	sb.WriteString("|----------|--------|----------|---------------|-------|\n")

	for _, r := range rg.results {
		status := "PASS"
		if !r.Success {
			status = "**FAIL**"
		}
		errMsg := "-"
		if r.Error != "" {
			errMsg = truncate(r.Error, 50)
		}
		sb.WriteString(fmt.Sprintf("| %s | %s | %s | %s | %s |\n",
			r.Scenario,
			status,
			r.Duration.Round(time.Millisecond),
			r.Metrics.RecoveryTime.Round(time.Millisecond),
			errMsg,
		))
	}
	sb.WriteString("\n")

	// Detailed results
	sb.WriteString("## Detailed Results\n\n")

	for _, r := range rg.results {
		sb.WriteString(fmt.Sprintf("### %s\n\n", r.Scenario))
		sb.WriteString(fmt.Sprintf("**Description:** %s\n\n", r.Description))

		status := "PASSED"
		if !r.Success {
			status = "FAILED"
		}
		sb.WriteString(fmt.Sprintf("**Status:** %s\n\n", status))

		if r.Error != "" {
			sb.WriteString(fmt.Sprintf("**Error:** %s\n\n", r.Error))
		}

		sb.WriteString("**Metrics:**\n\n")
		sb.WriteString(fmt.Sprintf("- Duration: %s\n", r.Duration.Round(time.Millisecond)))
		sb.WriteString(fmt.Sprintf("- Recovery Time: %s\n", r.Metrics.RecoveryTime.Round(time.Millisecond)))
		sb.WriteString(fmt.Sprintf("- Errors Before: %d\n", r.Metrics.ErrorsBeforeChaos))
		sb.WriteString(fmt.Sprintf("- Errors During: %d\n", r.Metrics.ErrorsDuringChaos))
		sb.WriteString(fmt.Sprintf("- Errors After: %d\n", r.Metrics.ErrorsAfterRecovery))
		sb.WriteString(fmt.Sprintf("- Data Loss: %v\n", r.Metrics.DataLoss))

		if r.Metrics.P99Latency > 0 {
			sb.WriteString(fmt.Sprintf("- P50 Latency: %s\n", r.Metrics.P50Latency.Round(time.Microsecond)))
			sb.WriteString(fmt.Sprintf("- P95 Latency: %s\n", r.Metrics.P95Latency.Round(time.Microsecond)))
			sb.WriteString(fmt.Sprintf("- P99 Latency: %s\n", r.Metrics.P99Latency.Round(time.Microsecond)))
		}
		sb.WriteString("\n")

		sb.WriteString("**Phases:**\n\n")
		for _, p := range r.Phases {
			status := "OK"
			if !p.Success {
				status = "FAIL"
			}
			sb.WriteString(fmt.Sprintf("- %s [%s] %s\n", p.Name, status, p.Duration.Round(time.Millisecond)))
			if p.Details != "" {
				sb.WriteString(fmt.Sprintf("  - %s\n", p.Details))
			}
			if p.Error != "" {
				sb.WriteString(fmt.Sprintf("  - Error: %s\n", p.Error))
			}
		}
		sb.WriteString("\n---\n\n")
	}

	_, err = f.WriteString(sb.String())
	return err
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

const htmlTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>{{.Title}}</title>
    <style>
        * {
            box-sizing: border-box;
            margin: 0;
            padding: 0;
        }
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Oxygen, Ubuntu, sans-serif;
            line-height: 1.6;
            color: #333;
            max-width: 1200px;
            margin: 0 auto;
            padding: 20px;
            background: #f5f5f5;
        }
        header {
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            color: white;
            padding: 30px;
            border-radius: 8px;
            margin-bottom: 20px;
        }
        h1 {
            font-size: 2rem;
            margin-bottom: 10px;
        }
        .timestamp {
            opacity: 0.9;
            font-size: 0.9rem;
        }
        .summary {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
            gap: 20px;
            margin-bottom: 30px;
        }
        .stat-card {
            background: white;
            padding: 20px;
            border-radius: 8px;
            box-shadow: 0 2px 4px rgba(0,0,0,0.1);
        }
        .stat-card h3 {
            color: #666;
            font-size: 0.85rem;
            text-transform: uppercase;
            margin-bottom: 5px;
        }
        .stat-card .value {
            font-size: 2rem;
            font-weight: bold;
            color: #333;
        }
        .stat-card.success .value { color: #22c55e; }
        .stat-card.failure .value { color: #ef4444; }
        .stat-card.warning .value { color: #f59e0b; }
        .results-table {
            width: 100%;
            background: white;
            border-radius: 8px;
            overflow: hidden;
            box-shadow: 0 2px 4px rgba(0,0,0,0.1);
            margin-bottom: 30px;
        }
        .results-table th,
        .results-table td {
            padding: 15px;
            text-align: left;
        }
        .results-table th {
            background: #f8f9fa;
            font-weight: 600;
            color: #555;
            text-transform: uppercase;
            font-size: 0.8rem;
        }
        .results-table tr:not(:last-child) {
            border-bottom: 1px solid #eee;
        }
        .status {
            display: inline-block;
            padding: 4px 12px;
            border-radius: 20px;
            font-size: 0.85rem;
            font-weight: 600;
        }
        .status.success {
            background: #dcfce7;
            color: #166534;
        }
        .status.failure {
            background: #fee2e2;
            color: #991b1b;
        }
        .scenario-detail {
            background: white;
            border-radius: 8px;
            padding: 20px;
            margin-bottom: 20px;
            box-shadow: 0 2px 4px rgba(0,0,0,0.1);
        }
        .scenario-detail h3 {
            margin-bottom: 15px;
            display: flex;
            align-items: center;
            gap: 10px;
        }
        .phases {
            margin-top: 15px;
        }
        .phase {
            display: flex;
            align-items: center;
            padding: 10px;
            border-left: 3px solid #ddd;
            margin: 10px 0;
            background: #f8f9fa;
        }
        .phase.success { border-color: #22c55e; }
        .phase.failure { border-color: #ef4444; }
        .phase-name {
            font-weight: 600;
            width: 100px;
        }
        .phase-duration {
            color: #666;
            margin-left: auto;
        }
        .metrics-grid {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(150px, 1fr));
            gap: 15px;
            margin-top: 15px;
        }
        .metric {
            padding: 10px;
            background: #f8f9fa;
            border-radius: 4px;
        }
        .metric-label {
            font-size: 0.8rem;
            color: #666;
            text-transform: uppercase;
        }
        .metric-value {
            font-size: 1.2rem;
            font-weight: 600;
        }
        .error-message {
            background: #fee2e2;
            color: #991b1b;
            padding: 10px;
            border-radius: 4px;
            margin-top: 10px;
            font-family: monospace;
            font-size: 0.9rem;
        }
    </style>
</head>
<body>
    <header>
        <h1>{{.Title}}</h1>
        <p class="timestamp">Generated: {{.Timestamp}}</p>
    </header>

    <section class="summary">
        <div class="stat-card">
            <h3>Total Scenarios</h3>
            <div class="value">{{.Summary.TotalScenarios}}</div>
        </div>
        <div class="stat-card success">
            <h3>Passed</h3>
            <div class="value">{{.Summary.PassedScenarios}}</div>
        </div>
        <div class="stat-card {{if gt .Summary.FailedScenarios 0}}failure{{end}}">
            <h3>Failed</h3>
            <div class="value">{{.Summary.FailedScenarios}}</div>
        </div>
        <div class="stat-card">
            <h3>Avg Recovery</h3>
            <div class="value">{{formatDuration .Summary.AvgRecoveryTime}}</div>
        </div>
        <div class="stat-card {{if gt .Summary.MaxRecoveryTime 120000000000}}warning{{end}}">
            <h3>Max Recovery</h3>
            <div class="value">{{formatDuration .Summary.MaxRecoveryTime}}</div>
        </div>
        <div class="stat-card {{if .Summary.DataLossDetected}}failure{{end}}">
            <h3>Data Loss</h3>
            <div class="value">{{if .Summary.DataLossDetected}}DETECTED{{else}}None{{end}}</div>
        </div>
    </section>

    <table class="results-table">
        <thead>
            <tr>
                <th>Scenario</th>
                <th>Status</th>
                <th>Duration</th>
                <th>Recovery Time</th>
                <th>Errors</th>
            </tr>
        </thead>
        <tbody>
            {{range .Results}}
            <tr>
                <td>{{.Scenario}}</td>
                <td><span class="status {{statusClass .Success}}">{{if .Success}}PASS{{else}}FAIL{{end}}</span></td>
                <td>{{formatDuration .Duration}}</td>
                <td>{{formatDuration .Metrics.RecoveryTime}}</td>
                <td>{{.Metrics.ErrorsDuringChaos}}</td>
            </tr>
            {{end}}
        </tbody>
    </table>

    <h2 style="margin-bottom: 20px;">Detailed Results</h2>

    {{range .Results}}
    <div class="scenario-detail">
        <h3>
            <span class="status {{statusClass .Success}}">{{statusIcon .Success}}</span>
            {{.Scenario}}
        </h3>
        <p>{{.Description}}</p>

        {{if .Error}}
        <div class="error-message">{{.Error}}</div>
        {{end}}

        <div class="metrics-grid">
            <div class="metric">
                <div class="metric-label">Duration</div>
                <div class="metric-value">{{formatDuration .Duration}}</div>
            </div>
            <div class="metric">
                <div class="metric-label">Recovery</div>
                <div class="metric-value">{{formatDuration .Metrics.RecoveryTime}}</div>
            </div>
            <div class="metric">
                <div class="metric-label">Errors Before</div>
                <div class="metric-value">{{.Metrics.ErrorsBeforeChaos}}</div>
            </div>
            <div class="metric">
                <div class="metric-label">Errors During</div>
                <div class="metric-value">{{.Metrics.ErrorsDuringChaos}}</div>
            </div>
            <div class="metric">
                <div class="metric-label">Errors After</div>
                <div class="metric-value">{{.Metrics.ErrorsAfterRecovery}}</div>
            </div>
            {{if gt .Metrics.P99Latency 0}}
            <div class="metric">
                <div class="metric-label">P99 Latency</div>
                <div class="metric-value">{{formatDuration .Metrics.P99Latency}}</div>
            </div>
            {{end}}
        </div>

        <div class="phases">
            <h4>Phases</h4>
            {{range .Phases}}
            <div class="phase {{statusClass .Success}}">
                <span class="phase-name">{{.Name}}</span>
                {{if .Details}}<span>{{.Details}}</span>{{end}}
                {{if .Error}}<span style="color: #991b1b;">{{.Error}}</span>{{end}}
                <span class="phase-duration">{{formatDuration .Duration}}</span>
            </div>
            {{end}}
        </div>
    </div>
    {{end}}
</body>
</html>`

// GenerateAllReports generates all report formats
func (rg *ReportGenerator) GenerateAllReports(basePath string) error {
	if err := rg.GenerateHTML(basePath + ".html"); err != nil {
		return fmt.Errorf("generate HTML report: %w", err)
	}

	if err := rg.GenerateJSON(basePath + ".json"); err != nil {
		return fmt.Errorf("generate JSON report: %w", err)
	}

	if err := rg.GenerateMarkdown(basePath + ".md"); err != nil {
		return fmt.Errorf("generate Markdown report: %w", err)
	}

	return nil
}
