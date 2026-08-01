package server

import (
	contracts "github.com/Parsaeffatravesh/tragge/packages/contracts/v1"
	"github.com/Parsaeffatravesh/tragge/packages/scoring"
)

// PositionSide represents the side of a position in the database ("long"/"short").
type PositionSide string

const (
	PositionSideLong  PositionSide = "long"
	PositionSideShort PositionSide = "short"
)

// OrderSideToPositionSide converts an order side (BUY/SELL) to a position side (long/short).
// BUY -> long, SELL -> short.
func OrderSideToPositionSide(s contracts.OrderSide) PositionSide {
	if s == contracts.OrderSideBuy {
		return PositionSideLong
	}
	return PositionSideShort
}

// PositionSideToOrderSide converts a DB position side string ("long"/"short") to an OrderSide.
// "long" -> BUY, anything else -> SELL.
// Also handles legacy "sell" values that may appear in recovery paths.
func PositionSideToOrderSide(s string) contracts.OrderSide {
	if s == "long" {
		return contracts.OrderSideBuy
	}
	return contracts.OrderSideSell
}

// OrderSideToDBOrderSide converts an order side (BUY/SELL) to the DB order side string ("buy"/"sell").
func OrderSideToDBOrderSide(s contracts.OrderSide) string {
	if s == contracts.OrderSideBuy {
		return "buy"
	}
	return "sell"
}

// DBOrderSideToOrderSide converts a DB order side string ("buy"/"sell") to an OrderSide.
// "buy" -> BUY, anything else -> SELL.
func DBOrderSideToOrderSide(s string) contracts.OrderSide {
	if s == "buy" {
		return contracts.OrderSideBuy
	}
	return contracts.OrderSideSell
}

// IsSameSide returns true if the DB position side and the order side represent the same direction.
// long == BUY, short == SELL.
func IsSameSide(dbPositionSide string, orderSide contracts.OrderSide) bool {
	return (dbPositionSide == "long" && orderSide == contracts.OrderSideBuy) ||
		(dbPositionSide == "short" && orderSide == contracts.OrderSideSell)
}

// IsLong returns true if the order side is BUY (i.e., a long position).
func IsLong(s contracts.OrderSide) bool {
	return s == contracts.OrderSideBuy
}

// OppositeSide returns the opposite order side: BUY -> SELL, SELL -> BUY.
func OppositeSide(s contracts.OrderSide) contracts.OrderSide {
	if s == contracts.OrderSideBuy {
		return contracts.OrderSideSell
	}
	return contracts.OrderSideBuy
}

// PositionSideToScoringSide converts a DB position side string to the scoring package's side constant.
func PositionSideToScoringSide(s string) string {
	if s == "long" {
		return scoring.SideLong
	}
	return scoring.SideShort
}
