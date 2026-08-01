package main

import (
	"fmt"

	"github.com/Parsaeffatravesh/tragge/packages/auth"
	"github.com/redis/go-redis/v9"
)

func loadAuthIsolationConfig(getenv, loadSecret func(string) string) (auth.IsolationConfig, error) {
	config := auth.LoadIsolationConfig(getenv("ENVIRONMENT"), getenv, loadSecret)
	if err := config.Validate(); err != nil {
		return auth.IsolationConfig{}, fmt.Errorf("authentication isolation validation failed: %w", err)
	}
	return config, nil
}

func buildAuthContexts(config auth.IsolationConfig, redisClient redis.UniversalClient) (*auth.Auth, *auth.Auth, error) {
	if err := config.Validate(); err != nil {
		return nil, nil, err
	}
	userAuth, err := auth.NewContext(config.User, redisClient)
	if err != nil {
		return nil, nil, fmt.Errorf("construct User authentication context: %w", err)
	}
	adminAuth, err := auth.NewContext(config.Admin, redisClient)
	if err != nil {
		return nil, nil, fmt.Errorf("construct Admin authentication context: %w", err)
	}
	if userAuth == adminAuth || userAuth.Context() != auth.ContextUser || adminAuth.Context() != auth.ContextAdmin {
		return nil, nil, fmt.Errorf("constructed authentication contexts are not isolated")
	}
	return userAuth, adminAuth, nil
}
