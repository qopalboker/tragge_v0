package server

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	contracts "github.com/Parsaeffatravesh/tragge/packages/contracts/v1"
	"github.com/Parsaeffatravesh/tragge/packages/infra"
	"github.com/twmb/franz-go/pkg/kgo"
	"go.uber.org/zap"
)

// consumeContestEvents reads contests.v1 topic to detect contest state changes
// and dynamically subscribe/unsubscribe symbols.
func (a *App) consumeContestEvents(ctx context.Context, topic string) {
	if a.dynamicSymbols == nil {
		return
	}

	brokers := a.config.KafkaBrokers
	if len(brokers) == 0 {
		a.log().Warn("no kafka brokers configured, contest event consumer disabled")
		return
	}

	opts := []kgo.Opt{
		kgo.SeedBrokers(brokers...),
		kgo.ConsumerGroup("market-ingestor-symbols"),
		kgo.ConsumeTopics(topic),
		kgo.ConsumeResetOffset(kgo.NewOffset().AtEnd()),
		kgo.SessionTimeout(30 * time.Second),
		kgo.RebalanceTimeout(30 * time.Second),
	}
	opts = append(opts, infra.KafkaSecurityOpts()...)
	client, err := kgo.NewClient(opts...)
	if err != nil {
		a.log().Error("failed to create contest event consumer", zap.Error(err))
		return
	}
	defer client.Close()

	a.log().Info("contest event consumer started",
		zap.String("topic", topic),
		zap.String("group", "market-ingestor-symbols"))

	for {
		fetches := client.PollFetches(ctx)
		if ctx.Err() != nil {
			return
		}

		fetches.EachRecord(func(record *kgo.Record) {
			var event contracts.ContestState
			if err := json.Unmarshal(record.Value, &event); err != nil {
				// Try extended format
				var ext contracts.ContestStateExtended
				if err2 := json.Unmarshal(record.Value, &ext); err2 != nil {
					a.log().Debug("failed to unmarshal contest event",
						zap.Error(err))
					return
				}
				event.ContestID = ext.ContestID
				event.Status = ext.Status
			}

			if event.ContestID == "" {
				return
			}

			status := string(event.Status)
			if status == "" {
				// Fall back to phase mapping
				switch event.Phase {
				case contracts.ContestPhaseUpcoming:
					status = "scheduled"
				case contracts.ContestPhaseLive:
					status = "running"
				case contracts.ContestPhaseEnded:
					status = "completed"
				case contracts.ContestPhaseCancelled:
					status = "cancelled"
				default:
					return
				}
			}

			a.log().Info("contest event received",
				zap.String("contest_id", event.ContestID),
				zap.String("status", status))

			a.dynamicSymbols.OnContestEvent(ctx, event.ContestID, status)
		})
	}
}

// handleDynamicSymbolStatus returns the currently active symbols managed by DynamicSymbolManager.
func (a *App) handleDynamicSymbolStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if a.dynamicSymbols == nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"enabled":        false,
			"active_symbols": []string{},
			"count":          0,
		})
		return
	}

	symbols := a.dynamicSymbols.GetActiveSymbols()
	json.NewEncoder(w).Encode(map[string]interface{}{
		"enabled":        true,
		"active_symbols": symbols,
		"count":          len(symbols),
	})
}
