package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"math/rand"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	contracts "github.com/Parsaeffatravesh/tragge/packages/contracts/v1"
	"github.com/twmb/franz-go/pkg/kgo"
)

// simulatedSymbol holds the state for price simulation
type simulatedSymbol struct {
	Symbol string
	Base   float64
	Spread float64
}

func main() {
	brokers := os.Getenv("KAFKA_BROKERS")
	if brokers == "" {
		brokers = "localhost:9092"
	}

	topic := os.Getenv("TICKS_TOPIC")
	if topic == "" {
		topic = "ticks.v1"
	}

	symbolsEnv := os.Getenv("SYMBOLS")
	if symbolsEnv == "" {
		symbolsEnv = "EUR/USD,GBP/USD,USD/JPY,AUD/USD,USD/CAD,XAU/USD,BTC/USD,ETH/USD"
	}

	// Base prices for common symbols
	basePrices := map[string]float64{
		"EUR/USD": 1.0850, "GBP/USD": 1.2650, "USD/JPY": 149.50,
		"AUD/USD": 0.6550, "USD/CAD": 1.3550, "USD/CHF": 0.8850,
		"NZD/USD": 0.6050, "EUR/GBP": 0.8580, "EUR/JPY": 162.20,
		"GBP/JPY": 189.10, "XAU/USD": 2320.0, "XAG/USD": 27.50,
		"BTC/USD": 67500.0, "ETH/USD": 3450.0, "SOL/USD": 145.0,
		"BNB/USD": 580.0, "XRP/USD": 0.5200, "DOGE/USD": 0.1250,
		"ADA/USD": 0.4500, "DOT/USD": 7.50,
	}

	symbols := []simulatedSymbol{}
	for _, s := range strings.Split(symbolsEnv, ",") {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		base, ok := basePrices[s]
		if !ok {
			base = 1.0
		}
		spread := base * 0.0002 // 2 pip spread
		if strings.Contains(s, "BTC") || strings.Contains(s, "ETH") {
			spread = base * 0.001 // wider spread for crypto
		}
		if strings.Contains(s, "XAU") {
			spread = 0.50
		}
		symbols = append(symbols, simulatedSymbol{Symbol: s, Base: base, Spread: spread})
	}

	client, err := kgo.NewClient(
		kgo.SeedBrokers(strings.Split(brokers, ",")...),
		kgo.DefaultProduceTopic(topic),
	)
	if err != nil {
		log.Fatalf("Failed to create Kafka client: %v", err)
	}
	defer client.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	log.Printf("Tick simulator started: %d symbols, topic=%s, brokers=%s", len(symbols), topic, brokers)

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	prices := make([]float64, len(symbols))
	for i, s := range symbols {
		prices[i] = s.Base
	}

	for {
		select {
		case <-quit:
			log.Println("Shutting down tick simulator...")
			return
		case <-ticker.C:
			now := time.Now().UnixMilli()
			ticks := make([]contracts.SymbolTick, 0, len(symbols))

			for i, s := range symbols {
				// Random walk with mean reversion
				change := (rand.Float64() - 0.5) * s.Base * 0.0005
				reversion := (s.Base - prices[i]) * 0.01
				prices[i] += change + reversion
				prices[i] = math.Max(prices[i]*0.95, math.Min(prices[i]*1.05, prices[i]))

				bid := prices[i]
				ask := bid + s.Spread
				last := bid + s.Spread/2

				ticks = append(ticks, contracts.SymbolTick{
					Symbol: s.Symbol,
					Bid:    math.Round(bid*100000) / 100000,
					Ask:    math.Round(ask*100000) / 100000,
					Last:   math.Round(last*100000) / 100000,
				})
			}

			snapshot := contracts.TickSnapshot{
				Ts:      now,
				Symbols: ticks,
			}

			data, err := json.Marshal(snapshot)
			if err != nil {
				log.Printf("Failed to marshal tick: %v", err)
				continue
			}

			results := client.ProduceSync(ctx, &kgo.Record{
				Topic: topic,
				Value: data,
			})
			if err := results.FirstErr(); err != nil {
				log.Printf("Failed to produce tick: %v", err)
				continue
			}

			if rand.Intn(10) == 0 {
				fmt.Printf("Published %d symbols (e.g. %s bid=%.5f)\n", len(ticks), ticks[0].Symbol, ticks[0].Bid)
			}
		}
	}
}
