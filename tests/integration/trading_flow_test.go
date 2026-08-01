package integration

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/Parsaeffatravesh/tragge/packages/auth"
	contracts "github.com/Parsaeffatravesh/tragge/packages/contracts/v1"
	"github.com/google/uuid"
	"github.com/twmb/franz-go/pkg/kgo"
)

// TradingTestEnv extends TestEnv with trading-specific helpers.
type TradingTestEnv struct {
	*TestEnv
	Auth *auth.Auth
}

// NewTradingTestEnv creates a new trading test environment.
func NewTradingTestEnv(t *testing.T, ctx context.Context) *TradingTestEnv {
	t.Helper()

	env := SetupTestEnv(t, ctx)

	authConfig := auth.DefaultConfig()
	authConfig.JWTSecret = env.JWTSecret
	authService := auth.New(authConfig)

	return &TradingTestEnv{
		TestEnv: env,
		Auth:    authService,
	}
}

// CreateTradingUser creates a user ready for trading.
func (te *TradingTestEnv) CreateTradingUser(ctx context.Context, t *testing.T, email string) (userID string, accessToken string) {
	t.Helper()

	passwordHash, err := te.Auth.HashPassword("testpassword123")
	if err != nil {
		t.Fatalf("Failed to hash password: %v", err)
	}

	userID = te.CreateTestUser(ctx, t, email, passwordHash)

	tokenPair, err := te.Auth.Token.GenerateTokenPair(userID, []string{"user"})
	if err != nil {
		t.Fatalf("Failed to generate token pair: %v", err)
	}

	return userID, tokenPair.AccessToken
}

// SetupTradingContest creates a contest with symbols ready for trading.
func (te *TradingTestEnv) SetupTradingContest(ctx context.Context, t *testing.T, name string, symbols []string) string {
	t.Helper()

	contestID := te.CreateTestContest(ctx, t, name, "running")

	for _, symbol := range symbols {
		te.AddContestSymbol(ctx, t, contestID, symbol)
	}

	return contestID
}

// PublishTickSnapshot publishes a tick snapshot to Kafka.
func (te *TradingTestEnv) PublishTickSnapshot(ctx context.Context, t *testing.T, snapshot *contracts.TickSnapshot) {
	t.Helper()

	data, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatalf("Failed to marshal tick snapshot: %v", err)
	}

	record := &kgo.Record{
		Topic: "ticks.v1",
		Key:   []byte("tick"),
		Value: data,
	}

	results := te.KafkaClient.ProduceSync(ctx, record)
	if err := results.FirstErr(); err != nil {
		t.Fatalf("Failed to publish tick snapshot: %v", err)
	}
}

// PublishOrderRequest publishes an order request to Kafka.
func (te *TradingTestEnv) PublishOrderRequest(ctx context.Context, t *testing.T, order *contracts.OrderRequest) {
	t.Helper()

	data, err := json.Marshal(order)
	if err != nil {
		t.Fatalf("Failed to marshal order request: %v", err)
	}

	record := &kgo.Record{
		Topic: "orders.v1",
		Key:   []byte(order.ContestID),
		Value: data,
	}

	results := te.KafkaClient.ProduceSync(ctx, record)
	if err := results.FirstErr(); err != nil {
		t.Fatalf("Failed to publish order request: %v", err)
	}
}

// ConsumeFillEvents consumes fill events from Kafka.
func (te *TradingTestEnv) ConsumeFillEvents(ctx context.Context, t *testing.T, timeout time.Duration) []contracts.FillEvent {
	t.Helper()

	consumer, err := kgo.NewClient(
		kgo.SeedBrokers(te.KafkaBrokers...),
		kgo.ConsumerGroup("test-fill-consumer-"+uuid.New().String()),
		kgo.ConsumeTopics("fills.v1"),
		kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()),
	)
	if err != nil {
		t.Fatalf("Failed to create fill consumer: %v", err)
	}
	defer consumer.Close()

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var fills []contracts.FillEvent

	for {
		fetches := consumer.PollFetches(ctx)
		if ctx.Err() != nil {
			break
		}

		fetches.EachRecord(func(record *kgo.Record) {
			var fill contracts.FillEvent
			if err := json.Unmarshal(record.Value, &fill); err != nil {
				t.Logf("Warning: Failed to unmarshal fill event: %v", err)
				return
			}
			fills = append(fills, fill)
		})

		if len(fills) > 0 {
			break
		}
	}

	return fills
}

// GetPosition retrieves a position from the database.
func (te *TradingTestEnv) GetPosition(ctx context.Context, t *testing.T, contestID, userID, symbol string) *Position {
	t.Helper()

	var pos Position
	err := te.DB.QueryRowContext(ctx, `
		SELECT position_id, contest_id, user_id, symbol, side, qty_open, entry_price, qty_used, realized_score
		FROM positions
		WHERE contest_id = $1 AND user_id = $2 AND symbol = $3 AND closed_at IS NULL
	`, contestID, userID, symbol).Scan(
		&pos.PositionID, &pos.ContestID, &pos.UserID, &pos.Symbol,
		&pos.Side, &pos.QtyOpen, &pos.EntryPrice, &pos.QtyUsed, &pos.RealizedScore,
	)
	if err != nil {
		return nil
	}

	return &pos
}

// Position represents a position in the database.
type Position struct {
	PositionID    string
	ContestID     string
	UserID        string
	Symbol        string
	Side          string
	QtyOpen       int64
	EntryPrice    float64
	QtyUsed       int64
	RealizedScore float64
}

// Order represents an order in the database.
type Order struct {
	OrderID    string
	ContestID  string
	UserID     string
	Symbol     string
	Side       string
	Type       string
	Qty        int64
	QtyFilled  int64
	LimitPrice *float64
	StopPrice  *float64
	TakeProfit *float64
	StopLoss   *float64
	Status     string
}

// GetOrder retrieves an order from the database.
func (te *TradingTestEnv) GetOrder(ctx context.Context, t *testing.T, orderID string) *Order {
	t.Helper()

	var order Order
	var limitPrice, stopPrice, takeProfit, stopLoss *float64

	err := te.DB.QueryRowContext(ctx, `
		SELECT order_id, contest_id, user_id, symbol, side, type, qty, qty_filled,
		       limit_price, stop_price, take_profit, stop_loss, status
		FROM orders
		WHERE order_id = $1
	`, orderID).Scan(
		&order.OrderID, &order.ContestID, &order.UserID, &order.Symbol,
		&order.Side, &order.Type, &order.Qty, &order.QtyFilled,
		&limitPrice, &stopPrice, &takeProfit, &stopLoss, &order.Status,
	)
	if err != nil {
		return nil
	}

	order.LimitPrice = limitPrice
	order.StopPrice = stopPrice
	order.TakeProfit = takeProfit
	order.StopLoss = stopLoss

	return &order
}

// GetFills retrieves fills for an order from the database.
func (te *TradingTestEnv) GetFills(ctx context.Context, t *testing.T, orderID string) []Fill {
	t.Helper()

	rows, err := te.DB.QueryContext(ctx, `
		SELECT fill_id, order_id, contest_id, user_id, symbol, side, qty, fill_price
		FROM fills
		WHERE order_id = $1
		ORDER BY created_at
	`, orderID)
	if err != nil {
		t.Fatalf("Failed to query fills: %v", err)
	}
	defer rows.Close()

	var fills []Fill
	for rows.Next() {
		var fill Fill
		if err := rows.Scan(
			&fill.FillID, &fill.OrderID, &fill.ContestID, &fill.UserID,
			&fill.Symbol, &fill.Side, &fill.Qty, &fill.FillPrice,
		); err != nil {
			t.Fatalf("Failed to scan fill: %v", err)
		}
		fills = append(fills, fill)
	}

	return fills
}

// Fill represents a fill in the database.
type Fill struct {
	FillID    string
	OrderID   string
	ContestID string
	UserID    string
	Symbol    string
	Side      string
	Qty       int64
	FillPrice float64
}

// GetParticipant retrieves a contest participant from the database.
func (te *TradingTestEnv) GetParticipant(ctx context.Context, t *testing.T, contestID, userID string) *Participant {
	t.Helper()

	var p Participant
	err := te.DB.QueryRowContext(ctx, `
		SELECT contest_id, user_id, qty_total, qty_available, total_score
		FROM contest_participants
		WHERE contest_id = $1 AND user_id = $2
	`, contestID, userID).Scan(&p.ContestID, &p.UserID, &p.QtyTotal, &p.QtyAvailable, &p.TotalScore)
	if err != nil {
		return nil
	}

	return &p
}

// Participant represents a contest participant in the database.
type Participant struct {
	ContestID    string
	UserID       string
	QtyTotal     int64
	QtyAvailable int64
	TotalScore   float64
}

// ============================================================================
// Integration Tests
// ============================================================================

func TestTradingFlow_JoinContestAndPlaceOrder(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Setup test environment
	te := NewTradingTestEnv(t, ctx)
	defer te.Cleanup(t, ctx)

	// Create test user
	userID, _ := te.CreateTradingUser(ctx, t, "trader@example.com")

	// Create test contest
	symbols := []string{"AAPL", "GOOGL", "MSFT"}
	contestID := te.SetupTradingContest(ctx, t, "Test Trading Contest", symbols)

	t.Run("JoinContest", func(t *testing.T) {
		// Join the contest
		te.JoinContest(ctx, t, contestID, userID, 100000)

		// Verify participant was created
		participant := te.GetParticipant(ctx, t, contestID, userID)
		if participant == nil {
			t.Fatal("Expected participant to be created")
		}

		if participant.QtyTotal != 100000 {
			t.Errorf("Expected qty_total 100000, got %d", participant.QtyTotal)
		}
		if participant.QtyAvailable != 100000 {
			t.Errorf("Expected qty_available 100000, got %d", participant.QtyAvailable)
		}
		if participant.TotalScore != 0 {
			t.Errorf("Expected total_score 0, got %f", participant.TotalScore)
		}
	})

	t.Run("PlaceMarketOrder_Validation", func(t *testing.T) {
		orderID := uuid.New().String()

		// Create order request
		order := &contracts.OrderRequest{
			OrderID:   orderID,
			UserID:    userID,
			ContestID: contestID,
			Symbol:    "AAPL",
			Side:      contracts.OrderSideBuy,
			Type:      contracts.OrderTypeMarket,
			Qty:       100,
			ClientTs:  time.Now().UnixMilli(),
		}

		// Publish order to Kafka
		te.PublishOrderRequest(ctx, t, order)

		// Give some time for the order to be processed
		time.Sleep(500 * time.Millisecond)

		// Verify order was published successfully
		// Note: We can't verify the order was processed without the trading engine running
		// This test just verifies the order can be published to Kafka
		t.Log("Market order published successfully")
	})

	t.Run("PlacePendingOrder_BuyLimit", func(t *testing.T) {
		orderID := uuid.New().String()
		limitPrice := 150.00

		// Create pending order request
		order := &contracts.OrderRequest{
			OrderID:    orderID,
			UserID:     userID,
			ContestID:  contestID,
			Symbol:     "AAPL",
			Side:       contracts.OrderSideBuy,
			Type:       contracts.OrderTypeBuyLimit,
			Qty:        50,
			LimitPrice: &limitPrice,
			ClientTs:   time.Now().UnixMilli(),
		}

		// Publish order to Kafka
		te.PublishOrderRequest(ctx, t, order)

		time.Sleep(500 * time.Millisecond)
		t.Log("Buy limit order published successfully")
	})

	t.Run("PlacePendingOrder_SellStop", func(t *testing.T) {
		orderID := uuid.New().String()
		stopPrice := 140.00

		// Create pending order request
		order := &contracts.OrderRequest{
			OrderID:   orderID,
			UserID:    userID,
			ContestID: contestID,
			Symbol:    "AAPL",
			Side:      contracts.OrderSideSell,
			Type:      contracts.OrderTypeSellStop,
			Qty:       25,
			StopPrice: &stopPrice,
			ClientTs:  time.Now().UnixMilli(),
		}

		// Publish order to Kafka
		te.PublishOrderRequest(ctx, t, order)

		time.Sleep(500 * time.Millisecond)
		t.Log("Sell stop order published successfully")
	})

	t.Run("PlaceOrderWithTPSL", func(t *testing.T) {
		orderID := uuid.New().String()
		takeProfit := 180.00
		stopLoss := 140.00

		// Create order with TP/SL
		order := &contracts.OrderRequest{
			OrderID:    orderID,
			UserID:     userID,
			ContestID:  contestID,
			Symbol:     "GOOGL",
			Side:       contracts.OrderSideBuy,
			Type:       contracts.OrderTypeMarket,
			Qty:        100,
			TakeProfit: &takeProfit,
			StopLoss:   &stopLoss,
			ClientTs:   time.Now().UnixMilli(),
		}

		// Publish order to Kafka
		te.PublishOrderRequest(ctx, t, order)

		time.Sleep(500 * time.Millisecond)
		t.Log("Order with TP/SL published successfully")
	})
}

func TestTradingFlow_TickSnapshot(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Setup test environment
	te := NewTradingTestEnv(t, ctx)
	defer te.Cleanup(t, ctx)

	t.Run("PublishAndConsumeTickSnapshot", func(t *testing.T) {
		// Create tick snapshot
		snapshot := &contracts.TickSnapshot{
			Ts: time.Now().UnixMilli(),
			Symbols: []contracts.SymbolTick{
				{Symbol: "AAPL", Bid: 174.50, Ask: 174.55, Last: 174.52},
				{Symbol: "GOOGL", Bid: 139.80, Ask: 139.85, Last: 139.82},
				{Symbol: "MSFT", Bid: 379.90, Ask: 380.00, Last: 379.95},
			},
		}

		// Publish tick snapshot
		te.PublishTickSnapshot(ctx, t, snapshot)

		// Create consumer to verify
		consumer, err := kgo.NewClient(
			kgo.SeedBrokers(te.KafkaBrokers...),
			kgo.ConsumerGroup("test-tick-consumer-"+uuid.New().String()),
			kgo.ConsumeTopics("ticks.v1"),
			kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()),
		)
		if err != nil {
			t.Fatalf("Failed to create consumer: %v", err)
		}
		defer consumer.Close()

		// Poll for messages
		ctx2, cancel2 := context.WithTimeout(ctx, 5*time.Second)
		defer cancel2()

		var receivedSnapshot *contracts.TickSnapshot
		for {
			fetches := consumer.PollFetches(ctx2)
			if ctx2.Err() != nil {
				break
			}

			fetches.EachRecord(func(record *kgo.Record) {
				var snap contracts.TickSnapshot
				if err := json.Unmarshal(record.Value, &snap); err == nil {
					receivedSnapshot = &snap
				}
			})

			if receivedSnapshot != nil && len(receivedSnapshot.Symbols) > 0 {
				break
			}
		}

		if receivedSnapshot == nil {
			t.Fatal("Expected to receive tick snapshot")
		}

		if len(receivedSnapshot.Symbols) != 3 {
			t.Errorf("Expected 3 symbols, got %d", len(receivedSnapshot.Symbols))
		}
	})
}

func TestTradingFlow_OrderTypes(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Setup test environment
	te := NewTradingTestEnv(t, ctx)
	defer te.Cleanup(t, ctx)

	t.Run("OrderType_IsPending", func(t *testing.T) {
		// Test order type helper methods
		if contracts.OrderTypeMarket.IsPending() {
			t.Error("MARKET orders should not be pending")
		}
		if !contracts.OrderTypeBuyLimit.IsPending() {
			t.Error("BUY_LIMIT orders should be pending")
		}
		if !contracts.OrderTypeSellLimit.IsPending() {
			t.Error("SELL_LIMIT orders should be pending")
		}
		if !contracts.OrderTypeBuyStop.IsPending() {
			t.Error("BUY_STOP orders should be pending")
		}
		if !contracts.OrderTypeSellStop.IsPending() {
			t.Error("SELL_STOP orders should be pending")
		}
	})

	t.Run("OrderType_GetMode", func(t *testing.T) {
		if contracts.OrderTypeMarket.GetMode() != contracts.OrderModeMarket {
			t.Error("MARKET orders should have MARKET mode")
		}
		if contracts.OrderTypeBuyLimit.GetMode() != contracts.OrderModePending {
			t.Error("BUY_LIMIT orders should have PENDING mode")
		}
		if contracts.OrderTypeSellLimit.GetMode() != contracts.OrderModePending {
			t.Error("SELL_LIMIT orders should have PENDING mode")
		}
	})
}

func TestTradingFlow_DatabaseOperations(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Setup test environment
	te := NewTradingTestEnv(t, ctx)
	defer te.Cleanup(t, ctx)

	// Create test user and contest
	userID, _ := te.CreateTradingUser(ctx, t, "dbtest@example.com")
	contestID := te.SetupTradingContest(ctx, t, "DB Test Contest", []string{"AAPL"})
	te.JoinContest(ctx, t, contestID, userID, 100000)

	t.Run("InsertOrder", func(t *testing.T) {
		orderID := uuid.New().String()
		limitPrice := 150.00

		_, err := te.DB.ExecContext(ctx, `
			INSERT INTO orders (order_id, contest_id, user_id, symbol, side, type, qty, limit_price, status)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		`, orderID, contestID, userID, "AAPL", "buy", "limit", 100, limitPrice, "pending")
		if err != nil {
			t.Fatalf("Failed to insert order: %v", err)
		}

		// Verify order was inserted
		order := te.GetOrder(ctx, t, orderID)
		if order == nil {
			t.Fatal("Expected order to be inserted")
		}

		if order.Symbol != "AAPL" {
			t.Errorf("Expected symbol AAPL, got %s", order.Symbol)
		}
		if order.Side != "buy" {
			t.Errorf("Expected side buy, got %s", order.Side)
		}
		if order.Qty != 100 {
			t.Errorf("Expected qty 100, got %d", order.Qty)
		}
		if order.Status != "pending" {
			t.Errorf("Expected status pending, got %s", order.Status)
		}
	})

	t.Run("UpdateOrderStatus", func(t *testing.T) {
		orderID := uuid.New().String()

		// Insert order
		_, err := te.DB.ExecContext(ctx, `
			INSERT INTO orders (order_id, contest_id, user_id, symbol, side, type, qty, status)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		`, orderID, contestID, userID, "AAPL", "buy", "market", 50, "pending")
		if err != nil {
			t.Fatalf("Failed to insert order: %v", err)
		}

		// Update status to filled
		_, err = te.DB.ExecContext(ctx, `
			UPDATE orders SET status = $1, qty_filled = qty WHERE order_id = $2
		`, "filled", orderID)
		if err != nil {
			t.Fatalf("Failed to update order: %v", err)
		}

		// Verify update
		order := te.GetOrder(ctx, t, orderID)
		if order.Status != "filled" {
			t.Errorf("Expected status filled, got %s", order.Status)
		}
		if order.QtyFilled != 50 {
			t.Errorf("Expected qty_filled 50, got %d", order.QtyFilled)
		}
	})

	t.Run("InsertFill", func(t *testing.T) {
		orderID := uuid.New().String()
		fillID := uuid.New().String()

		// Insert order first
		_, err := te.DB.ExecContext(ctx, `
			INSERT INTO orders (order_id, contest_id, user_id, symbol, side, type, qty, status)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		`, orderID, contestID, userID, "AAPL", "buy", "market", 100, "filled")
		if err != nil {
			t.Fatalf("Failed to insert order: %v", err)
		}

		// Insert fill
		_, err = te.DB.ExecContext(ctx, `
			INSERT INTO fills (fill_id, order_id, contest_id, user_id, symbol, side, qty, fill_price)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		`, fillID, orderID, contestID, userID, "AAPL", "buy", 100, 175.50)
		if err != nil {
			t.Fatalf("Failed to insert fill: %v", err)
		}

		// Verify fills
		fills := te.GetFills(ctx, t, orderID)
		if len(fills) != 1 {
			t.Fatalf("Expected 1 fill, got %d", len(fills))
		}

		fill := fills[0]
		if fill.FillPrice != 175.50 {
			t.Errorf("Expected fill price 175.50, got %f", fill.FillPrice)
		}
		if fill.Qty != 100 {
			t.Errorf("Expected qty 100, got %d", fill.Qty)
		}
	})

	t.Run("InsertPosition", func(t *testing.T) {
		positionID := uuid.New().String()

		// Insert position
		_, err := te.DB.ExecContext(ctx, `
			INSERT INTO positions (position_id, contest_id, user_id, symbol, side, qty_open, entry_price, qty_used)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		`, positionID, contestID, userID, "AAPL", "long", 100, 175.50, 17550)
		if err != nil {
			t.Fatalf("Failed to insert position: %v", err)
		}

		// Verify position
		pos := te.GetPosition(ctx, t, contestID, userID, "AAPL")
		if pos == nil {
			t.Fatal("Expected position to be created")
		}

		if pos.Side != "long" {
			t.Errorf("Expected side long, got %s", pos.Side)
		}
		if pos.QtyOpen != 100 {
			t.Errorf("Expected qty_open 100, got %d", pos.QtyOpen)
		}
		if pos.EntryPrice != 175.50 {
			t.Errorf("Expected entry_price 175.50, got %f", pos.EntryPrice)
		}
	})

	t.Run("UpdateParticipantScore", func(t *testing.T) {
		// Update total score
		_, err := te.DB.ExecContext(ctx, `
			UPDATE contest_participants SET total_score = $1 WHERE contest_id = $2 AND user_id = $3
		`, 5.25, contestID, userID)
		if err != nil {
			t.Fatalf("Failed to update participant: %v", err)
		}

		// Verify update
		participant := te.GetParticipant(ctx, t, contestID, userID)
		if participant.TotalScore != 5.25 {
			t.Errorf("Expected total_score 5.25, got %f", participant.TotalScore)
		}
	})
}

func TestTradingFlow_ContestLifecycle(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Setup test environment
	te := NewTradingTestEnv(t, ctx)
	defer te.Cleanup(t, ctx)

	t.Run("ContestStatusTransitions", func(t *testing.T) {
		// Create contest in draft status
		var contestID string
		err := te.DB.QueryRowContext(ctx, `
			INSERT INTO contests (name, starts_at, ends_at, status, qty_total)
			VALUES ($1, NOW() + INTERVAL '1 day', NOW() + INTERVAL '2 days', $2, 100000)
			RETURNING id
		`, "Lifecycle Test Contest", "draft").Scan(&contestID)
		if err != nil {
			t.Fatalf("Failed to create contest: %v", err)
		}

		// Transition to scheduled
		_, err = te.DB.ExecContext(ctx, `UPDATE contests SET status = 'scheduled' WHERE id = $1`, contestID)
		if err != nil {
			t.Fatalf("Failed to transition to scheduled: %v", err)
		}

		// Transition to registration_open
		_, err = te.DB.ExecContext(ctx, `UPDATE contests SET status = 'registration_open' WHERE id = $1`, contestID)
		if err != nil {
			t.Fatalf("Failed to transition to registration_open: %v", err)
		}

		// Create user and join
		userID, _ := te.CreateTradingUser(ctx, t, "lifecycle@example.com")
		te.JoinContest(ctx, t, contestID, userID, 100000)

		// Transition to running
		_, err = te.DB.ExecContext(ctx, `UPDATE contests SET status = 'running' WHERE id = $1`, contestID)
		if err != nil {
			t.Fatalf("Failed to transition to running: %v", err)
		}

		// Verify status
		var status string
		err = te.DB.QueryRowContext(ctx, `SELECT status FROM contests WHERE id = $1`, contestID).Scan(&status)
		if err != nil {
			t.Fatalf("Failed to query contest status: %v", err)
		}
		if status != "running" {
			t.Errorf("Expected status running, got %s", status)
		}

		// Transition to completed
		_, err = te.DB.ExecContext(ctx, `UPDATE contests SET status = 'completed' WHERE id = $1`, contestID)
		if err != nil {
			t.Fatalf("Failed to transition to completed: %v", err)
		}

		// Verify final status
		err = te.DB.QueryRowContext(ctx, `SELECT status FROM contests WHERE id = $1`, contestID).Scan(&status)
		if err != nil {
			t.Fatalf("Failed to query contest status: %v", err)
		}
		if status != "completed" {
			t.Errorf("Expected status completed, got %s", status)
		}
	})
}
