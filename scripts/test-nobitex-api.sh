#!/bin/bash
set -e

echo "=== Testing Nobitex API ==="

echo ""
echo "1. Testing /v3/orderbook/BTCUSDT"
curl -s "https://apiv2.nobitex.ir/v3/orderbook/BTCUSDT" | jq '{status, lastUpdate: .lastUpdate, lastTradePrice: .lastTradePrice, best_bid: .bids[0], best_ask: .asks[0]}'

echo ""
echo "2. Testing /v3/orderbook/all (first 3 symbols)"
curl -s "https://apiv2.nobitex.ir/v3/orderbook/all" | jq 'to_entries | map(select(.key != "status")) | .[0:3] | from_entries | to_entries[] | {symbol: .key, lastTradePrice: .value.lastTradePrice, bids_count: (.value.bids | length), asks_count: (.value.asks | length)}'

echo ""
echo "3. Checking which of our symbols exist"
SYMBOLS="BTCUSDT ETHUSDT SOLUSDT DOGEUSDT XRPUSDT ADAUSDT AVAXUSDT LINKUSDT DOTUSDT POLUSDT SHIBUSDT LTCUSDT UNIUSDT ETCUSDT XLMUSDT NEARUSDT AAVEUSDT SUIUSDT PEPEUSDT ARBUSDT OPUSDT APTUSDT INJUSDT RENDERUSDT"
RESPONSE=$(curl -s "https://apiv2.nobitex.ir/v3/orderbook/all")
for sym in $SYMBOLS; do
    EXISTS=$(echo "$RESPONSE" | jq -r "has(\"$sym\")")
    PRICE=$(echo "$RESPONSE" | jq -r ".[\"$sym\"].lastTradePrice // \"N/A\"")
    echo "  $sym: exists=$EXISTS price=$PRICE"
done

echo ""
echo "4. Testing /market/stats for BTC"
curl -s "https://apiv2.nobitex.ir/market/stats?srcCurrency=btc&dstCurrency=usdt" | jq '.stats["btc-usdt"] | {bestBuy, bestSell, latest, isClosed}'

echo ""
echo "=== Done ==="
