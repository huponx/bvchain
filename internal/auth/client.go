// Package auth logs in to the BV portal (pre-flight, CryptoJS AES password, TOTP) and returns a session.
package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

const (
	preflightURL = "https://api-hcm.banvien.com.vn/gatewayapp/auth/pre-flight"
	loginURL     = "https://api-hcm.banvien.com.vn/gatewayapp/auth"
)

type preflightRequest struct {
	Data string `json:"data"`
}

type preflightResponse struct {
	Code string `json:"code"`
	Data struct {
		Secret string `json:"secret"`
	} `json:"data"`
}

type loginRequest struct {
	Username   string `json:"username"`
	Password   string `json:"password"`
	IsExternal bool   `json:"is_external"`
	OTPCode    string `json:"otp"`
}

type loginResponse struct {
	AccessToken      string `json:"access_token"`
	RefreshToken     string `json:"refresh_token"`
	ExpiresIn        int    `json:"expires_in"`
	RefreshExpiresIn int    `json:"refresh_expires_in"`
	TokenType        string `json:"token_type"`
	NextStep         string `json:"next_step"`
}

// PreFlight calls the pre-flight API with the username and returns the secret.
func PreFlight(ctx context.Context, username string) (string, error) {
	body, err := json.Marshal(preflightRequest{Data: username})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, preflightURL, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("pre-flight request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var result preflightResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode pre-flight response: %w", err)
	}
	if result.Code != "CODE_SUCCESS" {
		return "", fmt.Errorf("pre-flight returned code %q", result.Code)
	}
	return result.Data.Secret, nil
}

// Login posts the encrypted password and the OTP code and returns the access token.
func Login(ctx context.Context, username, password, otpCode string) (string, error) {
	body, err := json.Marshal(loginRequest{Username: username, Password: password, IsExternal: false, OTPCode: otpCode})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, loginURL, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("login request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var result loginResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode login response: %w", err)
	}
	if result.AccessToken == "" {
		return "", fmt.Errorf("login returned no access token")
	}
	return result.AccessToken, nil
}
