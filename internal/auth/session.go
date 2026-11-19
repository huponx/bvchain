package auth

import (
	"context"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/huponx/bvchain/internal/crypto"
	"github.com/huponx/gocommon/jwtx"
	"github.com/huponx/gocommon/otp"
	"github.com/huponx/gocommon/retry"
	"go.uber.org/zap"
)

// Credentials are the BV account secrets.
type Credentials struct {
	Username  string
	Password  string
	OTPSecret string
}

// Session is a logged-in BV session.
type Session struct {
	Token  string
	UserID string
}

// GetSession runs the full flow (pre-flight, encrypt, OTP, login) and reads the user id from
// the access token. The token comes straight from the BV login API over TLS, so its payload
// is read without verifying a signature (jwtx.ParseUnverified): it is not used for any
// authorization decision here.
func GetSession(ctx context.Context, creds Credentials, log *zap.Logger) (Session, error) {
	log.Info("calling pre-flight", zap.String("username", creds.Username))
	var secret string
	if err := retry.Do(ctx, retry.Default, func() error {
		var err error
		secret, err = PreFlight(ctx, creds.Username)
		if err != nil {
			log.Warn("pre-flight failed, retrying", zap.Error(err))
		}
		return err
	}); err != nil {
		return Session{}, fmt.Errorf("pre-flight: %w", err)
	}

	secretBytes, err := base64.StdEncoding.DecodeString(secret)
	if err != nil {
		return Session{}, fmt.Errorf("decode secret: %w", err)
	}
	encrypted, err := crypto.EncryptCryptoJS(creds.Password, string(secretBytes))
	if err != nil {
		return Session{}, fmt.Errorf("encrypt password: %w", err)
	}

	code, err := otp.Generate(creds.OTPSecret, time.Now())
	if err != nil {
		return Session{}, fmt.Errorf("generate OTP: %w", err)
	}

	var token string
	if err := retry.Do(ctx, retry.Default, func() error {
		var err error
		token, err = Login(ctx, creds.Username, encrypted, code)
		if err != nil {
			log.Warn("login failed, retrying", zap.Error(err))
		}
		return err
	}); err != nil {
		return Session{}, fmt.Errorf("login: %w", err)
	}
	log.Info("login succeeded")

	var claims struct {
		UserID string `json:"userId"`
	}
	if err := jwtx.ParseUnverified(token, &claims); err != nil {
		return Session{}, fmt.Errorf("parse access token: %w", err)
	}
	if claims.UserID == "" {
		return Session{}, fmt.Errorf("access token has no userId")
	}
	return Session{Token: token, UserID: claims.UserID}, nil
}
