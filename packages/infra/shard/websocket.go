package shard

import (
	"fmt"
	"net/url"
)

// WebSocketEndpoint generates the WebSocket endpoint for a specific shard.
// This is used by trade-bff to route WebSocket connections to the correct
// trading-engine shard.
type WebSocketEndpoint struct {
	// WSPath is the WebSocket path on the trading-engine.
	// Example: "/ws"
	WSPath string

	// Scheme is the WebSocket scheme (ws or wss).
	Scheme string
}

// DefaultWebSocketEndpoint returns the default WebSocket endpoint configuration
// for Kubernetes StatefulSet deployments.
func DefaultWebSocketEndpoint() *WebSocketEndpoint {
	return &WebSocketEndpoint{
		Scheme: "ws",
		WSPath: "/ws",
	}
}

// GetEndpoint returns the WebSocket endpoint URL for a specific shard.
func (e *WebSocketEndpoint) GetEndpoint(shardInfo *ShardInfo) string {
	if shardInfo == nil {
		return ""
	}

	// Use the shard address directly if it's already a full address
	if shardInfo.Address != "" {
		return fmt.Sprintf("%s://%s%s", e.Scheme, shardInfo.Address, e.WSPath)
	}

	return ""
}

// GetEndpointWithQuery returns the WebSocket endpoint URL with query parameters.
func (e *WebSocketEndpoint) GetEndpointWithQuery(shardInfo *ShardInfo, params map[string]string) (string, error) {
	if shardInfo == nil {
		return "", fmt.Errorf("shard info is nil")
	}

	base := e.GetEndpoint(shardInfo)
	if base == "" {
		return "", fmt.Errorf("no endpoint available for shard")
	}

	u, err := url.Parse(base)
	if err != nil {
		return "", err
	}

	q := u.Query()
	for k, v := range params {
		q.Set(k, v)
	}
	u.RawQuery = q.Encode()

	return u.String(), nil
}

// ShardDiscoveryResponse represents the response for shard discovery endpoint.
type ShardDiscoveryResponse struct {
	ShardID     string `json:"shard_id"`
	WSEndpoint  string `json:"ws_endpoint"`
	HTTPAddress string `json:"http_address"`
	ContestID   string `json:"contest_id"`
}

// BuildDiscoveryResponse builds a discovery response for a shard.
func BuildDiscoveryResponse(shardInfo *ShardInfo, wsEndpoint *WebSocketEndpoint) *ShardDiscoveryResponse {
	if shardInfo == nil {
		return nil
	}

	return &ShardDiscoveryResponse{
		ShardID:     shardInfo.ShardID,
		WSEndpoint:  wsEndpoint.GetEndpoint(shardInfo),
		HTTPAddress: shardInfo.Address,
		ContestID:   shardInfo.ContestID,
	}
}

// TradingEngineAddress formats the trading-engine address for a given shard ID.
// This follows the Kubernetes StatefulSet naming convention:
// {statefulset-name}-{ordinal}.{headless-service-name}:{port}
func TradingEngineAddress(shardID string, headlessService string, port int) string {
	return fmt.Sprintf("trading-engine-%s.%s:%d", shardID, headlessService, port)
}
