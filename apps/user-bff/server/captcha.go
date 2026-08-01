package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const arcaptchaVerifyURL = "https://api.arcaptcha.co/arcaptcha/api/verify"

// captchaHTTPClient is a dedicated HTTP client for ARCaptcha API calls
// with sensible timeouts and connection pooling.
var captchaHTTPClient = &http.Client{
	Timeout: 15 * time.Second,
	Transport: &http.Transport{
		MaxIdleConns:    10,
		IdleConnTimeout: 30 * time.Second,
	},
}

type arcaptchaVerifyRequest struct {
	ChallengeID string `json:"challenge_id"`
	SiteKey     string `json:"site_key"`
	SecretKey   string `json:"secret_key"`
}

type arcaptchaVerifyResponse struct {
	Success bool `json:"success"`
}

// verifyCaptcha verifies an ARCaptcha token with the ARCaptcha API.
// Returns (true, nil) if verification passes, (false, nil) if rejected,
// or (false, err) if there was a communication error.
func verifyCaptcha(ctx context.Context, siteKey, secretKey, challengeID string) (bool, error) {
	reqBody := arcaptchaVerifyRequest{
		ChallengeID: challengeID,
		SiteKey:     siteKey,
		SecretKey:   secretKey,
	}

	bodyJSON, err := json.Marshal(reqBody)
	if err != nil {
		return false, fmt.Errorf("marshal captcha request: %w", err)
	}

	verifyCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(verifyCtx, http.MethodPost, arcaptchaVerifyURL, bytes.NewReader(bodyJSON))
	if err != nil {
		return false, fmt.Errorf("create captcha request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := captchaHTTPClient.Do(req)
	if err != nil {
		return false, fmt.Errorf("captcha API request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("captcha API returned status %d", resp.StatusCode)
	}

	var result arcaptchaVerifyResponse
	limitedBody := io.LimitReader(resp.Body, 1024) // max 1KB — response is tiny JSON
	if err := json.NewDecoder(limitedBody).Decode(&result); err != nil {
		return false, fmt.Errorf("decode captcha response: %w", err)
	}

	return result.Success, nil
}
