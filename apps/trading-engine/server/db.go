package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	contracts "github.com/Parsaeffatravesh/tragge/packages/contracts/v1"
	"github.com/shopspring/decimal"
)

// TxExecutor is an interface that can be either *sql.DB or *sql.Tx
type TxExecutor interface {
	ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error)
	QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row
	QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error)
}

// WithTransaction executes a function within a database transaction.
// If the function returns an error, the transaction is rolled back.
// Otherwise, the transaction is committed.
func WithTransaction(ctx context.Context, db *sql.DB, fn func(tx *sql.Tx) error) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}

	if err := fn(tx); err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			return fmt.Errorf("rollback failed: %v (original error: %w)", rbErr, err)
		}
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	return nil
}

// DBParticipant represents contest_participants row.
type DBParticipant struct {
	ContestID    string
	UserID       string
	QtyTotal     int64
	QtyAvailable int64
	TotalScore   float64
}

// DBPosition represents positions row.
type DBPosition struct {
	PositionID    string
	ContestID     string
	UserID        string
	Symbol        string
	Side          string // 'long' or 'short'
	QtyOpen       int64
	EntryPrice    float64
	QtyUsed       int64
	RealizedScore float64
}

// GetParticipant fetches a contest participant from the database.
func GetParticipant(ctx context.Context, db *sql.DB, contestID, userID string) (*DBParticipant, error) {
	row := db.QueryRowContext(ctx, `
		SELECT contest_id, user_id, qty_total, qty_available, total_score
		FROM contest_participants
		WHERE contest_id = $1 AND user_id = $2
	`, contestID, userID)

	p := &DBParticipant{}
	err := row.Scan(&p.ContestID, &p.UserID, &p.QtyTotal, &p.QtyAvailable, &p.TotalScore)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan participant: %w", err)
	}
	return p, nil
}

// GetContestParticipantCount returns the number of participants in a contest.
func GetContestParticipantCount(ctx context.Context, db *sql.DB, contestID string) (int, error) {
	var count int
	err := db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM contest_participants WHERE contest_id = $1
	`, contestID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count participants: %w", err)
	}
	return count, nil
}

// UpdateParticipantQtyAvailable updates the qty_available for a participant.
func UpdateParticipantQtyAvailable(ctx context.Context, db *sql.DB, contestID, userID string, qtyAvailable int64) error {
	return UpdateParticipantQtyAvailableTx(ctx, db, contestID, userID, qtyAvailable)
}

// UpdateParticipantQtyAvailableTx updates the qty_available for a participant (transaction-aware).
func UpdateParticipantQtyAvailableTx(ctx context.Context, tx TxExecutor, contestID, userID string, qtyAvailable int64) error {
	_, err := tx.ExecContext(ctx, `
		UPDATE contest_participants
		SET qty_available = $3
		WHERE contest_id = $1 AND user_id = $2
	`, contestID, userID, qtyAvailable)
	return err
}

// UpdateParticipantScore updates the total_score for a participant.
func UpdateParticipantScore(ctx context.Context, db *sql.DB, contestID, userID string, totalScore decimal.Decimal) error {
	return UpdateParticipantScoreTx(ctx, db, contestID, userID, totalScore)
}

// UpdateParticipantScoreTx updates the total_score for a participant (transaction-aware).
func UpdateParticipantScoreTx(ctx context.Context, tx TxExecutor, contestID, userID string, totalScore decimal.Decimal) error {
	_, err := tx.ExecContext(ctx, `
		UPDATE contest_participants
		SET total_score = $3
		WHERE contest_id = $1 AND user_id = $2
	`, contestID, userID, totalScore.String())
	return err
}

// UpdateParticipantQtyAndScoreTx atomically updates both qty_available and total_score.
func UpdateParticipantQtyAndScoreTx(ctx context.Context, tx TxExecutor, contestID, userID string, qtyAvailable int64, totalScore decimal.Decimal) error {
	_, err := tx.ExecContext(ctx, `
		UPDATE contest_participants
		SET qty_available = $3, total_score = $4
		WHERE contest_id = $1 AND user_id = $2
	`, contestID, userID, qtyAvailable, totalScore.String())
	return err
}

// InsertOrder inserts a new order into the database.
func InsertOrder(ctx context.Context, db *sql.DB, orderID, contestID, userID, symbol string,
	side contracts.OrderSide, orderType contracts.OrderType, qty int64,
	limitPrice, stopPrice, takeProfit, stopLoss *float64) error {

	dbSide := OrderSideToDBOrderSide(side)

	// Map order type to database enum.
	// Directional types are stored directly; legacy types kept for backward compatibility.
	dbType := "market"
	switch orderType {
	case contracts.OrderTypeBuyLimit:
		dbType = "buy_limit"
	case contracts.OrderTypeSellLimit:
		dbType = "sell_limit"
	case contracts.OrderTypeBuyStop:
		dbType = "buy_stop"
	case contracts.OrderTypeSellStop:
		dbType = "sell_stop"
	case contracts.OrderTypeLimit:
		dbType = "limit"
	case contracts.OrderTypeStop:
		dbType = "stop"
	}

	_, err := db.ExecContext(ctx, `
		INSERT INTO orders (order_id, contest_id, user_id, symbol, side, type, qty, limit_price, stop_price, take_profit, stop_loss, status)
		VALUES ($1, $2, $3, $4, $5::order_side, $6::order_type, $7, $8, $9, $10, $11, 'pending')
	`, orderID, contestID, userID, symbol, dbSide, dbType, qty, limitPrice, stopPrice, takeProfit, stopLoss)
	return err
}

// UpdateOrderStatus updates the status and qty_filled of an order.
func UpdateOrderStatus(ctx context.Context, db *sql.DB, orderID string, status string, qtyFilled int64) error {
	return UpdateOrderStatusTx(ctx, db, orderID, status, qtyFilled)
}

// UpdateOrderStatusTx updates the status and qty_filled of an order (transaction-aware).
func UpdateOrderStatusTx(ctx context.Context, tx TxExecutor, orderID string, status string, qtyFilled int64) error {
	_, err := tx.ExecContext(ctx, `
		UPDATE orders
		SET status = $2::order_status, qty_filled = $3
		WHERE order_id = $1
	`, orderID, status, qtyFilled)
	return err
}

// InsertFill inserts a fill record into the database.
func InsertFill(ctx context.Context, db *sql.DB, fillID, orderID, contestID, userID, symbol string,
	side contracts.OrderSide, qty int64, fillPrice float64) error {
	return InsertFillTx(ctx, db, fillID, orderID, contestID, userID, symbol, side, qty, fillPrice)
}

// InsertFillTx inserts a fill record into the database (transaction-aware).
func InsertFillTx(ctx context.Context, tx TxExecutor, fillID, orderID, contestID, userID, symbol string,
	side contracts.OrderSide, qty int64, fillPrice float64) error {

	dbSide := OrderSideToDBOrderSide(side)

	_, err := tx.ExecContext(ctx, `
		INSERT INTO fills (fill_id, order_id, contest_id, user_id, symbol, side, qty, fill_price, realized_pnl)
		VALUES ($1, $2, $3, $4, $5, $6::order_side, $7, $8, 0)
	`, fillID, orderID, contestID, userID, symbol, dbSide, qty, fillPrice)
	return err
}

// UpdateFillRealizedPnlTx updates the realized_pnl of a fill after position update determines the trade score.
func UpdateFillRealizedPnlTx(ctx context.Context, tx TxExecutor, fillID string, realizedPnl float64) error {
	_, err := tx.ExecContext(ctx, `
		UPDATE fills SET realized_pnl = $2 WHERE fill_id = $1
	`, fillID, realizedPnl)
	return err
}

// GetOpenPosition returns an open position for a user/symbol, or nil if not found.
func GetOpenPosition(ctx context.Context, db *sql.DB, contestID, userID, symbol string) (*DBPosition, error) {
	row := db.QueryRowContext(ctx, `
		SELECT position_id, contest_id, user_id, symbol, side, qty_open, entry_price, qty_used, realized_score
		FROM positions
		WHERE contest_id = $1 AND user_id = $2 AND symbol = $3 AND closed_at IS NULL
		ORDER BY opened_at DESC
		LIMIT 1
	`, contestID, userID, symbol)

	pos := &DBPosition{}
	err := row.Scan(&pos.PositionID, &pos.ContestID, &pos.UserID, &pos.Symbol,
		&pos.Side, &pos.QtyOpen, &pos.EntryPrice, &pos.QtyUsed, &pos.RealizedScore)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan position: %w", err)
	}
	return pos, nil
}

// GetOpenPositionTx returns an open position for a user/symbol within a transaction, or nil if not found.
// Uses FOR UPDATE to lock the row until the transaction commits/rolls back, preventing concurrent
// modifications (defense-in-depth alongside the in-memory PositionLockManager).
func GetOpenPositionTx(ctx context.Context, tx TxExecutor, contestID, userID, symbol string) (*DBPosition, error) {
	row := tx.QueryRowContext(ctx, `
		SELECT position_id, contest_id, user_id, symbol, side, qty_open, entry_price, qty_used, realized_score
		FROM positions
		WHERE contest_id = $1 AND user_id = $2 AND symbol = $3 AND closed_at IS NULL
		ORDER BY opened_at DESC
		LIMIT 1
		FOR UPDATE
	`, contestID, userID, symbol)

	pos := &DBPosition{}
	err := row.Scan(&pos.PositionID, &pos.ContestID, &pos.UserID, &pos.Symbol,
		&pos.Side, &pos.QtyOpen, &pos.EntryPrice, &pos.QtyUsed, &pos.RealizedScore)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan position: %w", err)
	}
	return pos, nil
}

// GetPositionByID returns a position by its ID, or nil if not found.
func GetPositionByID(ctx context.Context, db *sql.DB, positionID string) (*DBPosition, error) {
	row := db.QueryRowContext(ctx, `
		SELECT position_id, contest_id, user_id, symbol, side, qty_open, entry_price, qty_used, realized_score
		FROM positions
		WHERE position_id = $1 AND closed_at IS NULL
	`, positionID)

	pos := &DBPosition{}
	err := row.Scan(&pos.PositionID, &pos.ContestID, &pos.UserID, &pos.Symbol,
		&pos.Side, &pos.QtyOpen, &pos.EntryPrice, &pos.QtyUsed, &pos.RealizedScore)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan position: %w", err)
	}
	return pos, nil
}

// InsertPosition inserts a new position.
func InsertPosition(ctx context.Context, db *sql.DB, positionID, contestID, userID, symbol string,
	side string, qtyOpen int64, entryPrice float64, qtyUsed int64) error {
	return InsertPositionTx(ctx, db, positionID, contestID, userID, symbol, side, qtyOpen, entryPrice, qtyUsed)
}

// InsertPositionTx inserts a new position (transaction-aware).
func InsertPositionTx(ctx context.Context, tx TxExecutor, positionID, contestID, userID, symbol string,
	side string, qtyOpen int64, entryPrice float64, qtyUsed int64) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO positions (position_id, contest_id, user_id, symbol, side, qty_open, entry_price, qty_used, realized_score)
		VALUES ($1, $2, $3, $4, $5::position_side, $6, $7, $8, 0)
	`, positionID, contestID, userID, symbol, side, qtyOpen, entryPrice, qtyUsed)
	return err
}

// UpdatePosition updates an existing position.
func UpdatePosition(ctx context.Context, db *sql.DB, positionID string, qtyOpen int64, entryPrice float64, qtyUsed int64, realizedScore decimal.Decimal) error {
	return UpdatePositionTx(ctx, db, positionID, qtyOpen, entryPrice, qtyUsed, realizedScore)
}

// UpdatePositionTx updates an existing position (transaction-aware).
func UpdatePositionTx(ctx context.Context, tx TxExecutor, positionID string, qtyOpen int64, entryPrice float64, qtyUsed int64, realizedScore decimal.Decimal) error {
	_, err := tx.ExecContext(ctx, `
		UPDATE positions
		SET qty_open = $2, entry_price = $3, qty_used = $4, realized_score = $5
		WHERE position_id = $1
	`, positionID, qtyOpen, entryPrice, qtyUsed, realizedScore.String())
	return err
}

// ClosePosition marks a position as closed.
func ClosePosition(ctx context.Context, db *sql.DB, positionID string, realizedScore decimal.Decimal) error {
	return ClosePositionTx(ctx, db, positionID, realizedScore)
}

// ClosePositionTx marks a position as closed (transaction-aware).
func ClosePositionTx(ctx context.Context, tx TxExecutor, positionID string, realizedScore decimal.Decimal) error {
	_, err := tx.ExecContext(ctx, `
		UPDATE positions
		SET closed_at = NOW(), qty_open = 0, realized_score = $2
		WHERE position_id = $1
	`, positionID, realizedScore.String())
	return err
}

// UpdatePositionTPSL updates the take profit and stop loss levels for a position.
func UpdatePositionTPSL(ctx context.Context, db *sql.DB, positionID string, takeProfit, stopLoss *float64) error {
	return UpdatePositionTPSLTx(ctx, db, positionID, takeProfit, stopLoss)
}

// UpdatePositionTPSLTx updates the take profit and stop loss levels for a position (transaction-aware).
func UpdatePositionTPSLTx(ctx context.Context, tx TxExecutor, positionID string, takeProfit, stopLoss *float64) error {
	_, err := tx.ExecContext(ctx, `
		UPDATE positions
		SET take_profit = $2, stop_loss = $3, updated_at = NOW()
		WHERE position_id = $1 AND closed_at IS NULL
	`, positionID, takeProfit, stopLoss)
	return err
}

// GetAllOpenPositions returns all open positions for a user in a contest.
func GetAllOpenPositions(ctx context.Context, db *sql.DB, contestID, userID string) ([]*DBPosition, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT position_id, contest_id, user_id, symbol, side, qty_open, entry_price, qty_used, realized_score
		FROM positions
		WHERE contest_id = $1 AND user_id = $2 AND closed_at IS NULL
	`, contestID, userID)
	if err != nil {
		return nil, fmt.Errorf("query positions: %w", err)
	}
	defer rows.Close()

	var positions []*DBPosition
	for rows.Next() {
		pos := &DBPosition{}
		if err := rows.Scan(&pos.PositionID, &pos.ContestID, &pos.UserID, &pos.Symbol,
			&pos.Side, &pos.QtyOpen, &pos.EntryPrice, &pos.QtyUsed, &pos.RealizedScore); err != nil {
			return nil, fmt.Errorf("scan position row: %w", err)
		}
		positions = append(positions, pos)
	}
	return positions, rows.Err()
}

// ContestRules represents the rules_json configuration for a contest.
type ContestRules struct {
	// Price age thresholds in seconds (0 means use default from config)
	MaxPriceAgeMarketSeconds  *int `json:"max_price_age_market_seconds,omitempty"`
	MaxPriceAgePendingSeconds *int `json:"max_price_age_pending_seconds,omitempty"`
}

// DBContest represents a contest row.
type DBContest struct {
	ID         string
	Status     string
	AssetClass string
	StartsAt   sql.NullTime
	EndsAt     sql.NullTime
	Rules      *ContestRules
}

// GetContest fetches contest details from the database.
func GetContest(ctx context.Context, db *sql.DB, contestID string) (*DBContest, error) {
	row := db.QueryRowContext(ctx, `
		SELECT id, status, asset_class, starts_at, ends_at, rules_json
		FROM contests
		WHERE id = $1
	`, contestID)

	c := &DBContest{}
	var rulesJSON sql.NullString
	err := row.Scan(&c.ID, &c.Status, &c.AssetClass, &c.StartsAt, &c.EndsAt, &rulesJSON)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan contest: %w", err)
	}

	// Parse rules_json if present
	if rulesJSON.Valid && rulesJSON.String != "" {
		var rules ContestRules
		if err := json.Unmarshal([]byte(rulesJSON.String), &rules); err != nil {
			// Log but don't fail - just use defaults
			// We could log.Printf here but we're not importing log in db.go
			c.Rules = nil
		} else {
			c.Rules = &rules
		}
	}

	return c, nil
}

// GetContestSymbols returns the set of allowed symbols for a contest.
func GetContestSymbols(ctx context.Context, db *sql.DB, contestID string) (map[string]bool, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT symbol FROM contest_symbols WHERE contest_id = $1
	`, contestID)
	if err != nil {
		return nil, fmt.Errorf("query contest symbols: %w", err)
	}
	defer rows.Close()

	symbols := make(map[string]bool)
	for rows.Next() {
		var symbol string
		if err := rows.Scan(&symbol); err != nil {
			return nil, fmt.Errorf("scan symbol: %w", err)
		}
		symbols[symbol] = true
	}
	return symbols, rows.Err()
}

// GetAllPendingOrders returns all pending orders (status='pending') for recovery on startup.
func GetAllPendingOrders(ctx context.Context, db *sql.DB) ([]*PendingOrderInfo, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT order_id, contest_id, user_id, symbol, side, type, qty,
			   limit_price, stop_price, take_profit, stop_loss
		FROM orders
		WHERE status = 'pending'
		ORDER BY created_at ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("query pending orders: %w", err)
	}
	defer rows.Close()

	var orders []*PendingOrderInfo
	for rows.Next() {
		var (
			oid, cid, uid, symbol, side, otype string
			qty                                int64
			limitPrice, stopPrice              *float64
			takeProfit, stopLoss               *float64
		)
		if err := rows.Scan(&oid, &cid, &uid, &symbol, &side, &otype, &qty,
			&limitPrice, &stopPrice, &takeProfit, &stopLoss); err != nil {
			return nil, fmt.Errorf("scan pending order: %w", err)
		}

		orderSide := DBOrderSideToOrderSide(side)
		orderType := mapDBOrderType(otype, orderSide)

		orders = append(orders, &PendingOrderInfo{
			OrderID:    oid,
			ContestID:  cid,
			UserID:     uid,
			Symbol:     symbol,
			Side:       orderSide,
			Type:       orderType,
			Qty:        qty,
			LimitPrice: limitPrice,
			StopPrice:  stopPrice,
			TakeProfit: takeProfit,
			StopLoss:   stopLoss,
		})
	}
	return orders, rows.Err()
}

// mapDBOrderType maps database order type + side to the contracts OrderType.
// Handles both old (limit, stop) and new (buy_limit, sell_limit, buy_stop, sell_stop) DB values.
func mapDBOrderType(dbType string, side contracts.OrderSide) contracts.OrderType {
	switch dbType {
	case "buy_limit":
		return contracts.OrderTypeBuyLimit
	case "sell_limit":
		return contracts.OrderTypeSellLimit
	case "buy_stop":
		return contracts.OrderTypeBuyStop
	case "sell_stop":
		return contracts.OrderTypeSellStop
	case "limit":
		// Legacy: reconstruct directional type from side column
		if side == contracts.OrderSideBuy {
			return contracts.OrderTypeBuyLimit
		}
		return contracts.OrderTypeSellLimit
	case "stop":
		// Legacy: reconstruct directional type from side column
		if side == contracts.OrderSideBuy {
			return contracts.OrderTypeBuyStop
		}
		return contracts.OrderTypeSellStop
	default:
		return contracts.OrderTypeMarket
	}
}

// GetAllOpenPositionsWithTPSL returns all open positions that have TP or SL set.
func GetAllOpenPositionsWithTPSL(ctx context.Context, db *sql.DB) ([]*PositionWithTPSL, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT position_id, contest_id, user_id, symbol, side, qty_open, entry_price,
			   take_profit, stop_loss
		FROM positions
		WHERE closed_at IS NULL AND (take_profit IS NOT NULL OR stop_loss IS NOT NULL)
	`)
	if err != nil {
		return nil, fmt.Errorf("query positions with TP/SL: %w", err)
	}
	defer rows.Close()

	var positions []*PositionWithTPSL
	for rows.Next() {
		var (
			pid, cid, uid, symbol, side string
			qtyOpen                     int64
			entryPrice                  float64
			tp, sl                      *float64
		)
		if err := rows.Scan(&pid, &cid, &uid, &symbol, &side, &qtyOpen, &entryPrice, &tp, &sl); err != nil {
			return nil, fmt.Errorf("scan position: %w", err)
		}

		posSide := PositionSideToOrderSide(side)

		positions = append(positions, &PositionWithTPSL{
			PositionID: pid,
			ContestID:  cid,
			UserID:     uid,
			Symbol:     symbol,
			Side:       posSide,
			QtyOpen:    qtyOpen,
			EntryPrice: entryPrice,
			TakeProfit: tp,
			StopLoss:   sl,
		})
	}
	return positions, rows.Err()
}
