package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Parsaeffatravesh/tragge/packages/auth"
	contracts "github.com/Parsaeffatravesh/tragge/packages/contracts/v1"
	"github.com/Parsaeffatravesh/tragge/packages/validation"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/twmb/franz-go/pkg/kgo"
	"go.uber.org/zap"
)

// MeResponse is the response for /me endpoint
type MeResponse struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	Username  string    `json:"username"`
	CreatedAt time.Time `json:"created_at"`
	Roles     []string  `json:"roles"`
}

// OrderHistoryItem represents a single order in the history response
type OrderHistoryItem struct {
	OrderID   string     `json:"order_id"`
	Symbol    string     `json:"symbol"`
	Side      string     `json:"side"`
	Type      string     `json:"type"`
	Qty       int64      `json:"qty"`
	Status    string     `json:"status"`
	CreatedAt time.Time  `json:"created_at"`
	FillPrice *float64   `json:"fill_price,omitempty"`
	FillQty   *int64     `json:"fill_qty,omitempty"`
	FillTime  *time.Time `json:"fill_time,omitempty"`
	Pnl       float64    `json:"pnl"`
}

// OrderHistoryResponse is the response for GET /api/trade/orders/history
type OrderHistoryResponse struct {
	Orders []OrderHistoryItem `json:"orders"`
	Total  int                `json:"total"`
	Limit  int                `json:"limit"`
	Offset int                `json:"offset"`
}

// handleMe handles GET /api/trade/me
func (a *App) handleMe(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := auth.GetUserID(ctx)
	roles := auth.GetRoles(ctx)

	// Get user from database (read-only, use replica)
	var email string
	var createdAt time.Time
	err := a.pool.Replica().QueryRowContext(ctx,
		`SELECT email, created_at FROM users WHERE id = $1`,
		userID,
	).Scan(&email, &createdAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": tradeMsg.UserNotFound})
			return
		}
		a.log().Error("Failed to query user", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": tradeMsg.InternalError})
		return
	}

	// Generate username from email (part before @)
	username := email
	if atIndex := strings.Index(email, "@"); atIndex > 0 {
		username = email[:atIndex]
	}

	writeJSON(w, http.StatusOK, MeResponse{
		ID:        userID,
		Email:     email,
		Username:  username,
		CreatedAt: createdAt,
		Roles:     roles,
	})
}

// contestInfo holds contest status and time window information.
type contestInfo struct {
	Status   string
	StartsAt sql.NullTime
	EndsAt   sql.NullTime
}

// getContestStatus retrieves the status and time window for a contest.
func (a *App) getContestStatus(ctx context.Context, contestID string) (*contestInfo, error) {
	var info contestInfo
	err := a.pool.Replica().QueryRowContext(ctx,
		`SELECT status, starts_at, ends_at FROM contests WHERE id = $1`,
		contestID,
	).Scan(&info.Status, &info.StartsAt, &info.EndsAt)
	if err != nil {
		return nil, err
	}
	return &info, nil
}

// validateContestRunning checks that a contest exists and is running.
// If checkTimeWindow is true, also validates starts_at/ends_at.
// Returns true if contest is valid and running, false if an error response was written.
func (a *App) validateContestRunning(w http.ResponseWriter, ctx context.Context, contestID string, checkTimeWindow bool) bool {
	info, err := a.getContestStatus(ctx, contestID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": tradeMsg.ContestNotFound})
			return false
		}
		a.log().Error("Failed to get contest", zap.Error(err), zap.String("contest_id", contestID))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": tradeMsg.InternalError})
		return false
	}
	if info.Status != "running" {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": tradeMsg.ContestNotRunning})
		return false
	}
	if checkTimeWindow {
		now := time.Now()
		if info.StartsAt.Valid && now.Before(info.StartsAt.Time) {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": tradeMsg.ContestNotStarted})
			return false
		}
		if info.EndsAt.Valid && now.After(info.EndsAt.Time) {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": tradeMsg.ContestEnded})
			return false
		}
	}
	return true
}


// OrderSubmitRequest represents the request body for order placement
type OrderSubmitRequest struct {
	ContestID  string              `json:"contest_id"`
	Symbol     string              `json:"symbol"`
	Side       contracts.OrderSide `json:"side"`
	Type       contracts.OrderType `json:"type"`
	Qty        int64               `json:"qty"`
	LimitPrice *float64            `json:"limit_price,omitempty"`
	StopPrice  *float64            `json:"stop_price,omitempty"`
	TakeProfit *float64            `json:"take_profit,omitempty"`
	StopLoss   *float64            `json:"stop_loss,omitempty"`
}

// handlePlaceOrder handles POST /api/trade/orders
func (a *App) handlePlaceOrder(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Get authenticated user from context
	userID := auth.GetUserID(ctx)
	if userID == "" {
		validation.WriteUnauthorized(w, "unauthorized")
		return
	}

	// Parse request body
	var req OrderSubmitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		validation.WriteBadRequest(w, "invalid request body")
		return
	}

	// Validate and sanitize fields using validation package
	v := validation.New()
	req.ContestID = v.UUID("contest_id", req.ContestID)
	req.Symbol = v.Symbol("symbol", req.Symbol)
	v.Quantity("qty", req.Qty, validation.DefaultQuantityConstraints())

	// Validate optional price fields
	if req.LimitPrice != nil {
		v.PricePtr("limit_price", req.LimitPrice, validation.DefaultPriceConstraints())
	}
	if req.StopPrice != nil {
		v.PricePtr("stop_price", req.StopPrice, validation.DefaultPriceConstraints())
	}
	if req.TakeProfit != nil {
		v.PricePtr("take_profit", req.TakeProfit, validation.DefaultPriceConstraints())
	}
	if req.StopLoss != nil {
		v.PricePtr("stop_loss", req.StopLoss, validation.DefaultPriceConstraints())
	}

	if v.HasErrors() {
		validation.WriteValidationError(w, v.Errors())
		return
	}

	// Validate order side
	if req.Side != contracts.OrderSideBuy && req.Side != contracts.OrderSideSell {
		validation.WriteBadRequest(w, "side must be BUY or SELL")
		return
	}

	// Validate contest is running and within time window
	if !a.validateContestRunning(w, ctx, req.ContestID, true) {
		return
	}

	// Validate user is joined to contest (read-only, use replica)
	var exists bool
	err := a.pool.Replica().QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM contest_participants WHERE contest_id = $1 AND user_id = $2)`,
		req.ContestID, userID,
	).Scan(&exists)
	if err != nil {
		a.log().Error("Failed to check contest participation", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": tradeMsg.InternalError})
		return
	}
	if !exists {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": tradeMsg.NotParticipant})
		return
	}

	// Generate order ID
	orderID := uuid.New().String()

	// Build OrderRequest
	orderReq := contracts.OrderRequest{
		OrderID:    orderID,
		UserID:     userID,
		ContestID:  req.ContestID,
		Symbol:     req.Symbol,
		Side:       req.Side,
		Type:       req.Type,
		Qty:        req.Qty,
		LimitPrice: req.LimitPrice,
		StopPrice:  req.StopPrice,
		TakeProfit: req.TakeProfit,
		StopLoss:   req.StopLoss,
		ClientTs:   time.Now().UnixMilli(),
	}

	// Serialize to JSON
	data, err := json.Marshal(orderReq)
	if err != nil {
		a.log().Error("Failed to marshal order request", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": tradeMsg.InternalError})
		return
	}

	// Publish to Kafka with contest_id as partition key
	// This ensures all orders for a contest go to the same partition,
	// maintaining order and enabling shard-based routing to trading-engine instances.
	// See docs/kafka-partitioning.md for details.
	if a.ordersKafka != nil {
		record := &kgo.Record{
			Topic: a.config.OrdersTopic,
			Key:   []byte(req.ContestID), // Partition key for contest-local ordering
			Value: data,
		}
		results := a.ordersKafka.ProduceSync(ctx, record)
		if err := results.FirstErr(); err != nil {
			a.log().Error("Failed to publish order to Kafka", zap.Error(err))
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": tradeMsg.OrderSubmitFailed})
			return
		}
		a.log().Info("Order published to Kafka",
			zap.String("order_id", orderID),
			zap.String("user_id", userID),
			zap.String("contest_id", req.ContestID),
			zap.String("symbol", req.Symbol))
	} else {
		a.log().Warn("Kafka producer not available, order not published", zap.String("order_id", orderID))
	}

	// Return 202 Accepted with order_id
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]string{"order_id": orderID})
}

// handleCancelOrder handles DELETE /api/trade/orders/{order_id}
func (a *App) handleCancelOrder(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Get authenticated user from context
	userID := auth.GetUserID(ctx)
	if userID == "" {
		validation.WriteUnauthorized(w, "unauthorized")
		return
	}

	// Get order_id from URL path
	orderID := chi.URLParam(r, "order_id")
	if orderID == "" {
		validation.WriteBadRequest(w, "order_id is required")
		return
	}

	// Validate order_id is a valid UUID
	v := validation.New()
	orderID = v.UUID("order_id", orderID)
	if v.HasErrors() {
		validation.WriteValidationError(w, v.Errors())
		return
	}

	// Get order details from database
	var contestID, symbol, orderStatus, orderUserID string
	var qty int64

	err := a.pool.Replica().QueryRowContext(ctx,
		`SELECT contest_id, user_id, symbol, qty, status
		 FROM orders WHERE order_id = $1`,
		orderID,
	).Scan(&contestID, &orderUserID, &symbol, &qty, &orderStatus)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": tradeMsg.OrderNotFound})
			return
		}
		a.log().Error("Failed to get order", zap.Error(err), zap.String("order_id", orderID))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": tradeMsg.InternalError})
		return
	}

	// Verify order belongs to authenticated user
	if orderUserID != userID {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": tradeMsg.OrderNotYours})
		return
	}

	// Check if order is already cancelled (idempotent - return success)
	if orderStatus == "cancelled" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(map[string]string{
			"message":  tradeMsg.OrderCancelSubmitted,
			"order_id": orderID,
		})
		return
	}

	// Check if order status is 'pending' (can only cancel pending orders)
	if orderStatus != "pending" {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": tradeMsg.CannotCancelOrder,
		})
		return
	}

	// Verify contest is still running
	if !a.validateContestRunning(w, ctx, contestID, false) {
		return
	}

	// Build CancelOrderRequest
	cancelReq := contracts.CancelOrderRequest{
		OrderID:      orderID,
		UserID:       userID,
		ContestID:    contestID,
		Symbol:       symbol,
		Qty:          qty,
		CancelReason: contracts.CancelReasonUserRequested,
		ClientTs:     time.Now().UnixMilli(),
	}

	// Serialize to JSON
	data, err := json.Marshal(cancelReq)
	if err != nil {
		a.log().Error("Failed to marshal cancel order request", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": tradeMsg.InternalError})
		return
	}

	// Publish to Kafka cancel orders topic with contest_id as partition key
	if a.ordersKafka != nil {
		record := &kgo.Record{
			Topic: a.config.CancelOrdersTopic,
			Key:   []byte(contestID),
			Value: data,
		}
		results := a.ordersKafka.ProduceSync(ctx, record)
		if err := results.FirstErr(); err != nil {
			a.log().Error("Failed to publish cancel order to Kafka", zap.Error(err))
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": tradeMsg.CancelSubmitFailed})
			return
		}
		a.log().Info("Cancel order published to Kafka",
			zap.String("order_id", orderID),
			zap.String("user_id", userID),
			zap.String("contest_id", contestID),
			zap.String("symbol", symbol))
	} else {
		a.log().Warn("Kafka producer not available, cancel order not published", zap.String("order_id", orderID))
	}

	// Return 202 Accepted
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]string{
		"message":  tradeMsg.OrderCancelSubmitted,
		"order_id": orderID,
	})
}

// ClosePositionAPIRequest represents the optional request body for closing a position
type ClosePositionAPIRequest struct {
	Qty *int64 `json:"qty,omitempty"` // Optional: quantity to close (if omitted, close entire position)
}

// handleClosePosition handles POST /api/trade/positions/{position_id}/close
func (a *App) handleClosePosition(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Get authenticated user from context
	userID := auth.GetUserID(ctx)
	if userID == "" {
		validation.WriteUnauthorized(w, "unauthorized")
		return
	}

	// Get position_id from URL path
	positionID := chi.URLParam(r, "position_id")
	if positionID == "" {
		validation.WriteBadRequest(w, "position_id is required")
		return
	}

	// Validate position_id is a valid UUID
	v := validation.New()
	positionID = v.UUID("position_id", positionID)
	if v.HasErrors() {
		validation.WriteValidationError(w, v.Errors())
		return
	}

	// Parse optional request body for partial close
	var req ClosePositionAPIRequest
	if r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			validation.WriteBadRequest(w, "invalid request body")
			return
		}
	}

	// Get position details from database
	var contestID, symbol, sideStr string
	var qtyOpen int64
	var closedAt sql.NullTime
	var posUserID string

	err := a.pool.Replica().QueryRowContext(ctx,
		`SELECT contest_id, user_id, symbol, side, qty_open, closed_at
		 FROM positions WHERE position_id = $1`,
		positionID,
	).Scan(&contestID, &posUserID, &symbol, &sideStr, &qtyOpen, &closedAt)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			validation.WriteNotFound(w, "position not found")
			return
		}
		a.log().Error("Failed to get position", zap.Error(err), zap.String("position_id", positionID))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": tradeMsg.InternalError})
		return
	}

	// Verify position belongs to authenticated user
	if posUserID != userID {
		validation.WriteForbidden(w, "position does not belong to user")
		return
	}

	// Check if position is already closed
	if closedAt.Valid || qtyOpen <= 0 {
		validation.WriteBadRequest(w, "position is already closed")
		return
	}

	// Verify contest is running
	if !a.validateContestRunning(w, ctx, contestID, false) {
		return
	}

	// Determine quantity to close
	qtyToClose := qtyOpen // Default: close entire position
	if req.Qty != nil {
		if *req.Qty <= 0 {
			validation.WriteBadRequest(w, "qty must be greater than 0")
			return
		}
		if *req.Qty > qtyOpen {
			validation.WriteBadRequest(w, "close quantity exceeds position size")
			return
		}
		qtyToClose = *req.Qty
	}

	// Build ClosePositionRequest
	closeReq := contracts.ClosePositionRequest{
		PositionID:  positionID,
		UserID:      userID,
		ContestID:   contestID,
		Symbol:      symbol,
		Side:        contracts.OrderSide(sideStr),
		QtyToClose:  qtyToClose,
		CloseReason: contracts.CloseReasonUserRequested,
		ClientTs:    time.Now().UnixMilli(),
	}

	// Serialize to JSON
	data, err := json.Marshal(closeReq)
	if err != nil {
		a.log().Error("Failed to marshal close position request", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": tradeMsg.InternalError})
		return
	}

	// Publish to Kafka close_positions topic with contest_id as partition key
	if a.ordersKafka != nil {
		record := &kgo.Record{
			Topic: a.config.ClosePositionsTopic,
			Key:   []byte(contestID),
			Value: data,
		}
		results := a.ordersKafka.ProduceSync(ctx, record)
		if err := results.FirstErr(); err != nil {
			a.log().Error("Failed to publish close position to Kafka", zap.Error(err))
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": tradeMsg.ClosePositionFailed})
			return
		}
		a.log().Info("Close position published to Kafka",
			zap.String("position_id", positionID),
			zap.String("user_id", userID),
			zap.String("contest_id", contestID),
			zap.String("symbol", symbol),
			zap.Int64("qty_to_close", qtyToClose),
			zap.String("topic", a.config.ClosePositionsTopic))
	} else {
		a.log().Warn("Kafka producer not available, close position not published", zap.String("position_id", positionID))
	}

	// Return 202 Accepted
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]string{
		"position_id": positionID,
		"message":     tradeMsg.ClosePositionSubmitted,
	})
}

// ModifyTPSLAPIRequest represents the request body for modifying TP/SL levels
type ModifyTPSLAPIRequest struct {
	TakeProfit *float64 `json:"take_profit"` // null to remove TP
	StopLoss   *float64 `json:"stop_loss"`   // null to remove SL
}

// handleModifyTPSL handles PUT /api/trade/positions/{position_id}/tpsl
func (a *App) handleModifyTPSL(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Get authenticated user from context
	userID := auth.GetUserID(ctx)
	if userID == "" {
		validation.WriteUnauthorized(w, "unauthorized")
		return
	}

	// Get position_id from URL path
	positionID := chi.URLParam(r, "position_id")
	if positionID == "" {
		validation.WriteBadRequest(w, "position_id is required")
		return
	}

	// Validate position_id is a valid UUID
	v := validation.New()
	positionID = v.UUID("position_id", positionID)
	if v.HasErrors() {
		validation.WriteValidationError(w, v.Errors())
		return
	}

	// Parse request body
	var req ModifyTPSLAPIRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		validation.WriteBadRequest(w, "invalid request body")
		return
	}

	// Validate TP/SL values are positive if provided
	priceConstraints := validation.DefaultPriceConstraints()
	if req.TakeProfit != nil {
		v.PricePtr("take_profit", req.TakeProfit, priceConstraints)
	}
	if req.StopLoss != nil {
		v.PricePtr("stop_loss", req.StopLoss, priceConstraints)
	}
	if v.HasErrors() {
		validation.WriteValidationError(w, v.Errors())
		return
	}

	// TP and SL cannot be equal if both are set
	if req.TakeProfit != nil && req.StopLoss != nil && *req.TakeProfit == *req.StopLoss {
		validation.WriteBadRequest(w, "take_profit and stop_loss cannot be equal")
		return
	}

	// Get position details from database
	var contestID, symbol, sideStr, posUserID string
	var entryPrice float64
	var qtyOpen int64
	var closedAt sql.NullTime

	err := a.pool.Replica().QueryRowContext(ctx,
		`SELECT contest_id, user_id, symbol, side, entry_price, qty_open, closed_at
		 FROM positions WHERE position_id = $1`,
		positionID,
	).Scan(&contestID, &posUserID, &symbol, &sideStr, &entryPrice, &qtyOpen, &closedAt)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			validation.WriteNotFound(w, "position not found")
			return
		}
		a.log().Error("Failed to get position", zap.Error(err), zap.String("position_id", positionID))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": tradeMsg.InternalError})
		return
	}

	// Verify position belongs to authenticated user
	if posUserID != userID {
		validation.WriteForbidden(w, "position does not belong to user")
		return
	}

	// Check if position is open
	if closedAt.Valid || qtyOpen <= 0 {
		validation.WriteBadRequest(w, "position is already closed")
		return
	}

	// Verify contest is running
	if !a.validateContestRunning(w, ctx, contestID, false) {
		return
	}

	// Validate TP/SL values based on position side
	// LONG (side=BUY): TP > entry_price, SL < entry_price
	// SHORT (side=SELL): TP < entry_price, SL > entry_price
	side := strings.ToUpper(sideStr)
	if side == "BUY" {
		// LONG position
		if req.TakeProfit != nil && *req.TakeProfit <= entryPrice {
			validation.WriteBadRequest(w, "for long positions, take_profit must be greater than entry price")
			return
		}
		if req.StopLoss != nil && *req.StopLoss >= entryPrice {
			validation.WriteBadRequest(w, "for long positions, stop_loss must be less than entry price")
			return
		}
	} else if side == "SELL" {
		// SHORT position
		if req.TakeProfit != nil && *req.TakeProfit >= entryPrice {
			validation.WriteBadRequest(w, "for short positions, take_profit must be less than entry price")
			return
		}
		if req.StopLoss != nil && *req.StopLoss <= entryPrice {
			validation.WriteBadRequest(w, "for short positions, stop_loss must be greater than entry price")
			return
		}
	}

	// Build ModifyTPSLRequest
	modifyReq := contracts.ModifyTPSLRequest{
		PositionID: positionID,
		UserID:     userID,
		ContestID:  contestID,
		Symbol:     symbol,
		TakeProfit: req.TakeProfit,
		StopLoss:   req.StopLoss,
		ClientTs:   time.Now().UnixMilli(),
	}

	// Serialize to JSON
	data, err := json.Marshal(modifyReq)
	if err != nil {
		a.log().Error("Failed to marshal modify TPSL request", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": tradeMsg.InternalError})
		return
	}

	// Publish to Kafka modify_tpsl topic with contest_id as partition key
	if a.ordersKafka != nil {
		record := &kgo.Record{
			Topic: a.config.ModifyTPSLTopic,
			Key:   []byte(contestID),
			Value: data,
		}
		results := a.ordersKafka.ProduceSync(ctx, record)
		if err := results.FirstErr(); err != nil {
			a.log().Error("Failed to publish modify TPSL to Kafka", zap.Error(err))
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": tradeMsg.TPSLUpdateFailed})
			return
		}
		a.log().Info("Modify TPSL published to Kafka",
			zap.String("position_id", positionID),
			zap.String("user_id", userID),
			zap.String("contest_id", contestID),
			zap.String("symbol", symbol),
			zap.String("topic", a.config.ModifyTPSLTopic),
			zap.Any("take_profit", req.TakeProfit),
			zap.Any("stop_loss", req.StopLoss))
	} else {
		a.log().Warn("Kafka producer not available, modify TPSL not published", zap.String("position_id", positionID))
	}

	// Return 200 OK
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"message": tradeMsg.TPSLUpdated,
	})
}

// handleOrderHistory handles GET /api/trade/orders/history
// Returns paginated order history for the authenticated user
func (a *App) handleOrderHistory(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Get authenticated user from context
	userID := auth.GetUserID(ctx)
	if userID == "" {
		validation.WriteUnauthorized(w, "unauthorized")
		return
	}

	// Parse query parameters
	q := r.URL.Query()

	// contest_id is required
	contestID := q.Get("contest_id")
	if contestID == "" {
		validation.WriteBadRequest(w, "contest_id is required")
		return
	}

	// Validate contest_id is a valid UUID
	v := validation.New()
	contestID = v.UUID("contest_id", contestID)
	if v.HasErrors() {
		validation.WriteValidationError(w, v.Errors())
		return
	}

	// Parse limit (default 50, max 100)
	limit := 50
	if l := q.Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			if parsed > 100 {
				limit = 100
			} else {
				limit = parsed
			}
		}
	}

	// Parse offset (default 0)
	offset := 0
	if o := q.Get("offset"); o != "" {
		if parsed, err := strconv.Atoi(o); err == nil && parsed >= 0 {
			offset = parsed
		}
	}

	// Parse status filter (optional: "filled", "cancelled", "all")
	statusFilter := q.Get("status")
	if statusFilter != "" && statusFilter != "filled" && statusFilter != "cancelled" && statusFilter != "all" {
		validation.WriteBadRequest(w, "status must be 'filled', 'cancelled', or 'all'")
		return
	}

	// Verify user is a participant in this contest
	var participantExists bool
	err := a.pool.Replica().QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM contest_participants WHERE contest_id = $1 AND user_id = $2)`,
		contestID, userID,
	).Scan(&participantExists)
	if err != nil {
		a.log().Error("Failed to check contest participation", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": tradeMsg.InternalError})
		return
	}
	if !participantExists {
		validation.WriteForbidden(w, "user is not a participant in this contest")
		return
	}

	// Build status filter condition
	var statusCondition string
	baseArgs := []interface{}{contestID, userID}

	switch statusFilter {
	case "filled":
		statusCondition = " AND o.status = $3"
		baseArgs = append(baseArgs, "filled")
	case "cancelled":
		statusCondition = " AND o.status = $3"
		baseArgs = append(baseArgs, "cancelled")
	case "all", "":
		// No additional filter - return all statuses
		statusCondition = ""
	}

	// Count query for pagination
	countQuery := `
		SELECT COUNT(*)
		FROM orders o
		WHERE o.contest_id = $1 AND o.user_id = $2` + statusCondition

	var total int
	if err := a.pool.Replica().QueryRowContext(ctx, countQuery, baseArgs...).Scan(&total); err != nil {
		a.log().Error("Failed to count orders", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": tradeMsg.InternalError})
		return
	}

	// Main query with LEFT JOIN to fills
	// We aggregate fill data for each order (an order may have multiple fills)
	limitParam := len(baseArgs) + 1
	offsetParam := len(baseArgs) + 2
	query := `
		SELECT
			o.order_id,
			o.symbol,
			UPPER(o.side::text) as side,
			UPPER(o.type::text) as type,
			o.qty,
			o.status::text,
			o.created_at,
			SUM(f.fill_price * f.qty) / NULLIF(SUM(f.qty), 0) as avg_fill_price,
			SUM(f.qty) as total_fill_qty,
			MAX(f.created_at) as last_fill_time,
			COALESCE(SUM(f.realized_pnl), 0) as pnl
		FROM orders o
		LEFT JOIN fills f ON o.order_id = f.order_id
		WHERE o.contest_id = $1 AND o.user_id = $2` + statusCondition + `
		GROUP BY o.order_id, o.symbol, o.side, o.type, o.qty, o.status, o.created_at
		ORDER BY o.created_at DESC
		LIMIT $` + strconv.Itoa(limitParam) + ` OFFSET $` + strconv.Itoa(offsetParam)

	rows, err := a.pool.Replica().QueryContext(ctx, query, append(baseArgs, limit, offset)...)
	if err != nil {
		a.log().Error("Failed to query order history", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": tradeMsg.InternalError})
		return
	}
	defer rows.Close()

	orders := make([]OrderHistoryItem, 0)
	for rows.Next() {
		var item OrderHistoryItem
		var avgFillPrice sql.NullFloat64
		var totalFillQty sql.NullInt64
		var lastFillTime sql.NullTime
		var pnl sql.NullFloat64

		if err := rows.Scan(
			&item.OrderID,
			&item.Symbol,
			&item.Side,
			&item.Type,
			&item.Qty,
			&item.Status,
			&item.CreatedAt,
			&avgFillPrice,
			&totalFillQty,
			&lastFillTime,
			&pnl,
		); err != nil {
			a.log().Error("Failed to scan order row", zap.Error(err))
			continue
		}

		// Set fill fields if available
		if avgFillPrice.Valid {
			item.FillPrice = &avgFillPrice.Float64
		}
		if totalFillQty.Valid {
			item.FillQty = &totalFillQty.Int64
		}
		if lastFillTime.Valid {
			item.FillTime = &lastFillTime.Time
		}

		if pnl.Valid {
			item.Pnl = pnl.Float64
		}

		orders = append(orders, item)
	}

	if err := rows.Err(); err != nil {
		a.log().Error("Error iterating order rows", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": tradeMsg.InternalError})
		return
	}

	writeJSON(w, http.StatusOK, OrderHistoryResponse{
		Orders: orders,
		Total:  total,
		Limit:  limit,
		Offset: offset,
	})
}

// BalanceResponse represents the user's QTY/balance info for a contest
type BalanceResponse struct {
	ContestID    string `json:"contest_id"`
	UserID       string `json:"user_id"`
	QtyTotal     int64  `json:"qty_total"`     // Total QTY allocated (from contest config)
	QtyAvailable int64  `json:"qty_available"` // Available for new trades
	QtyUsed      int64  `json:"qty_used"`      // Currently in use (total - available)
}

// handleGetBalance handles GET /api/trade/balance
// Returns the user's QTY allocation for a specific contest
func (a *App) handleGetBalance(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Get authenticated user from context
	userID := auth.GetUserID(ctx)
	if userID == "" {
		validation.WriteUnauthorized(w, "unauthorized")
		return
	}

	// contest_id is required
	contestID := r.URL.Query().Get("contest_id")
	if contestID == "" {
		validation.WriteBadRequest(w, "contest_id is required")
		return
	}

	// Validate contest_id is a valid UUID
	v := validation.New()
	contestID = v.UUID("contest_id", contestID)
	if v.HasErrors() {
		validation.WriteValidationError(w, v.Errors())
		return
	}

	// Query participant's QTY info (read-only, use replica)
	var qtyTotal, qtyAvailable int64
	err := a.pool.Replica().QueryRowContext(ctx,
		`SELECT qty_total, qty_available FROM contest_participants WHERE contest_id = $1 AND user_id = $2`,
		contestID, userID,
	).Scan(&qtyTotal, &qtyAvailable)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			validation.WriteForbidden(w, "user is not a participant in this contest")
			return
		}
		a.log().Error("Failed to get participant balance", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": tradeMsg.InternalError})
		return
	}

	writeJSON(w, http.StatusOK, BalanceResponse{
		ContestID:    contestID,
		UserID:       userID,
		QtyTotal:     qtyTotal,
		QtyAvailable: qtyAvailable,
		QtyUsed:      qtyTotal - qtyAvailable,
	})
}
