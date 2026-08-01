#!/bin/bash
# Common environment variables for running Go services locally
# Sources secrets from Docker secret files

SECRETS_DIR="infra/docker/secrets"

export POSTGRES_HOST=localhost
export POSTGRES_PORT=5432
export POSTGRES_DB=app
export POSTGRES_SSLMODE=disable
export POSTGRES_USER=tragge_app
export POSTGRES_PASSWORD=$(cat "$SECRETS_DIR/postgres_app_password.txt")

export REDIS_ADDR=localhost:6379
export REDIS_MODE=standalone
export REDIS_PASSWORD=$(cat "$SECRETS_DIR/redis_password.txt")

export KAFKA_BROKERS=localhost:9092

export JWT_SECRET=$(cat "$SECRETS_DIR/jwt_secret.txt")
export JWT_REFRESH_SECRET=$(cat "$SECRETS_DIR/jwt_refresh_secret.txt" 2>/dev/null || echo "$JWT_SECRET")

export ENVIRONMENT=development
export NOTIFICATION_ENABLED=false
export NOBITEX_ENABLED=true
export NOBITEX_POLL_INTERVAL=5s
export MARKET_PROVIDER=twelvedata
export TWELVEDATA_API_KEYS=$(cat "$SECRETS_DIR/twelvedata_api_keys.txt")
export NOBITEX_TOKEN=$(cat "$SECRETS_DIR/nobitex_token.txt" 2>/dev/null)
export CONTROL_API_KEY=1a3fbd860658126a01399860e45217be779f28cc5f696d2ddc5f18e5c2b0c859

export ALLOWED_ORIGINS="http://localhost:8080,https://cautious-happiness-4j4wgrg4w796cp7r-8080.app.github.dev,http://localhost:5173,http://localhost:5174,http://localhost:5175,https://cautious-happiness-4j4wgrg4w796cp7r-5173.app.github.dev,https://cautious-happiness-4j4wgrg4w796cp7r-5174.app.github.dev,https://cautious-happiness-4j4wgrg4w796cp7r-5175.app.github.dev,https://cautious-happiness-4j4wgrg4w796cp7r-5176.app.github.dev,https://cautious-happiness-4j4wgrg4w796cp7r-5177.app.github.dev,https://cautious-happiness-4j4wgrg4w796cp7r-5178.app.github.dev"
export CORS_ALLOWED_ORIGINS="http://localhost:8080,https://cautious-happiness-4j4wgrg4w796cp7r-8080.app.github.dev,http://localhost:5173,http://localhost:5174,http://localhost:5175,https://cautious-happiness-4j4wgrg4w796cp7r-5173.app.github.dev,https://cautious-happiness-4j4wgrg4w796cp7r-5174.app.github.dev,https://cautious-happiness-4j4wgrg4w796cp7r-5175.app.github.dev,https://cautious-happiness-4j4wgrg4w796cp7r-5176.app.github.dev,https://cautious-happiness-4j4wgrg4w796cp7r-5177.app.github.dev,https://cautious-happiness-4j4wgrg4w796cp7r-5178.app.github.dev"
export GOOGLE_REDIRECT_URI=http://localhost:8080/user/auth/google/callback
