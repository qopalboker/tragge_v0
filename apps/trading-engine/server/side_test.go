package server

import (
	"testing"

	contracts "github.com/Parsaeffatravesh/tragge/packages/contracts/v1"
	"github.com/Parsaeffatravesh/tragge/packages/scoring"
)

func TestOrderSideToPositionSide(t *testing.T) {
	tests := []struct {
		input contracts.OrderSide
		want  PositionSide
	}{
		{contracts.OrderSideBuy, PositionSideLong},
		{contracts.OrderSideSell, PositionSideShort},
	}
	for _, tt := range tests {
		got := OrderSideToPositionSide(tt.input)
		if got != tt.want {
			t.Errorf("OrderSideToPositionSide(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestPositionSideToOrderSide(t *testing.T) {
	tests := []struct {
		input string
		want  contracts.OrderSide
	}{
		{"long", contracts.OrderSideBuy},
		{"short", contracts.OrderSideSell},
		{"sell", contracts.OrderSideSell},  // legacy value
		{"other", contracts.OrderSideSell}, // unknown defaults to SELL
	}
	for _, tt := range tests {
		got := PositionSideToOrderSide(tt.input)
		if got != tt.want {
			t.Errorf("PositionSideToOrderSide(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestOrderSideToDBOrderSide(t *testing.T) {
	if got := OrderSideToDBOrderSide(contracts.OrderSideBuy); got != "buy" {
		t.Errorf("OrderSideToDBOrderSide(BUY) = %q, want %q", got, "buy")
	}
	if got := OrderSideToDBOrderSide(contracts.OrderSideSell); got != "sell" {
		t.Errorf("OrderSideToDBOrderSide(SELL) = %q, want %q", got, "sell")
	}
}

func TestDBOrderSideToOrderSide(t *testing.T) {
	tests := []struct {
		input string
		want  contracts.OrderSide
	}{
		{"buy", contracts.OrderSideBuy},
		{"sell", contracts.OrderSideSell},
		{"other", contracts.OrderSideSell},
	}
	for _, tt := range tests {
		got := DBOrderSideToOrderSide(tt.input)
		if got != tt.want {
			t.Errorf("DBOrderSideToOrderSide(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestIsSameSide(t *testing.T) {
	tests := []struct {
		dbSide    string
		orderSide contracts.OrderSide
		want      bool
	}{
		{"long", contracts.OrderSideBuy, true},
		{"short", contracts.OrderSideSell, true},
		{"long", contracts.OrderSideSell, false},
		{"short", contracts.OrderSideBuy, false},
	}
	for _, tt := range tests {
		got := IsSameSide(tt.dbSide, tt.orderSide)
		if got != tt.want {
			t.Errorf("IsSameSide(%q, %q) = %v, want %v", tt.dbSide, tt.orderSide, got, tt.want)
		}
	}
}

func TestIsLong(t *testing.T) {
	if !IsLong(contracts.OrderSideBuy) {
		t.Error("IsLong(BUY) should be true")
	}
	if IsLong(contracts.OrderSideSell) {
		t.Error("IsLong(SELL) should be false")
	}
}

func TestOppositeSide(t *testing.T) {
	if got := OppositeSide(contracts.OrderSideBuy); got != contracts.OrderSideSell {
		t.Errorf("OppositeSide(BUY) = %q, want SELL", got)
	}
	if got := OppositeSide(contracts.OrderSideSell); got != contracts.OrderSideBuy {
		t.Errorf("OppositeSide(SELL) = %q, want BUY", got)
	}
}

func TestPositionSideToScoringSide(t *testing.T) {
	if got := PositionSideToScoringSide("long"); got != scoring.SideLong {
		t.Errorf("PositionSideToScoringSide(long) = %q, want %q", got, scoring.SideLong)
	}
	if got := PositionSideToScoringSide("short"); got != scoring.SideShort {
		t.Errorf("PositionSideToScoringSide(short) = %q, want %q", got, scoring.SideShort)
	}
}

func TestRoundTrip_OrderSide_PositionSide(t *testing.T) {
	// BUY -> long -> BUY
	pos := OrderSideToPositionSide(contracts.OrderSideBuy)
	back := PositionSideToOrderSide(string(pos))
	if back != contracts.OrderSideBuy {
		t.Errorf("Round trip BUY->long->BUY failed: got %q", back)
	}

	// SELL -> short -> SELL
	pos = OrderSideToPositionSide(contracts.OrderSideSell)
	back = PositionSideToOrderSide(string(pos))
	if back != contracts.OrderSideSell {
		t.Errorf("Round trip SELL->short->SELL failed: got %q", back)
	}
}

func TestRoundTrip_OrderSide_DBOrderSide(t *testing.T) {
	// BUY -> buy -> BUY
	db := OrderSideToDBOrderSide(contracts.OrderSideBuy)
	back := DBOrderSideToOrderSide(db)
	if back != contracts.OrderSideBuy {
		t.Errorf("Round trip BUY->buy->BUY failed: got %q", back)
	}

	// SELL -> sell -> SELL
	db = OrderSideToDBOrderSide(contracts.OrderSideSell)
	back = DBOrderSideToOrderSide(db)
	if back != contracts.OrderSideSell {
		t.Errorf("Round trip SELL->sell->SELL failed: got %q", back)
	}
}
