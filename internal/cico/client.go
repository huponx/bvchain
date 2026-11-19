package cico

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

const (
	cicoURL = "https://api-hcm.banvien.com.vn/gatewayapp/ci-co"

	defaultAddress   = "E.Town Central, 11, Đoàn Văn Bơ, Phường Xóm Chiếu, Thủ Đức, Ho Chi Minh City, 72806, Vietnam"
	defaultLatitude  = 10.764701082174815
	defaultLongitude = 106.70300995347357
)

type cicoRequest struct {
	Address   string  `json:"address"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Type      int     `json:"type"`
}

type cicoResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Record calls the CI-CO API
func Record(ctx context.Context, sessionToken string) error {
	body, err := json.Marshal(cicoRequest{
		Address:   defaultAddress,
		Latitude:  defaultLatitude,
		Longitude: defaultLongitude,
		Type:      2,
	})
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cicoURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+sessionToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("ci-co request failed: %w", err)
	}
	defer resp.Body.Close()

	var result cicoResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("decode ci-co response: %w", err)
	}

	if result.Code != "CODE_SUCCESS" {
		return fmt.Errorf("ci-co returned code %q: %s", result.Code, result.Message)
	}

	return nil
}
