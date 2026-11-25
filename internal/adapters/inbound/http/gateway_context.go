package http

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"os"
	"strings"

	"github.com/google/uuid"
)

// withGatewayContext extracts gateway headers and verified JWT identity claims
// into the downstream request context.
func withGatewayContext(r *http.Request) context.Context {
	ctx := r.Context()
	if auth := r.Header.Get("Authorization"); auth != "" {
		ctx = context.WithValue(ctx, "authorization", auth)
	}
	if orgID := resolveOrganizationID(r); orgID != uuid.Nil {
		ctx = context.WithValue(ctx, "organization_id", orgID.String())
	}
	if userID := resolveUserID(r); userID != "" {
		ctx = context.WithValue(ctx, "user_id", userID)
	}
	return ctx
}

// resolveOrganizationID resolves a tenant organization UUID in priority order:
// 1. X-Organization-ID request header (forwarded by API gateway or client)
// 2. Non-nil fallback UUIDs supplied by the caller (e.g. JSON body organization_id)
// 3. Verified Bearer JWT organization_id / org_id claim
func resolveOrganizationID(r *http.Request, fallbacks ...uuid.UUID) uuid.UUID {
	if r != nil {
		if orgID, err := uuid.Parse(strings.TrimSpace(r.Header.Get("X-Organization-ID"))); err == nil && orgID != uuid.Nil {
			return orgID
		}
	}
	for _, fb := range fallbacks {
		if fb != uuid.Nil {
			return fb
		}
	}
	claims := parseVerifiedBearerClaims(r)
	for _, key := range []string{"organization_id", "org_id"} {
		if raw, ok := claims[key].(string); ok {
			if orgID, err := uuid.Parse(strings.TrimSpace(raw)); err == nil && orgID != uuid.Nil {
				return orgID
			}
		}
	}
	return uuid.Nil
}

func resolveUserID(r *http.Request) string {
	if r == nil {
		return ""
	}
	if userID := strings.TrimSpace(r.Header.Get("X-User-ID")); userID != "" {
		return userID
	}
	claims := parseVerifiedBearerClaims(r)
	for _, key := range []string{"user_id", "sub", "id"} {
		if raw, ok := claims[key].(string); ok && strings.TrimSpace(raw) != "" {
			return strings.TrimSpace(raw)
		}
	}
	return ""
}

func parseVerifiedBearerClaims(r *http.Request) map[string]interface{} {
	if r == nil {
		return nil
	}
	authHeader := strings.TrimSpace(r.Header.Get("Authorization"))
	if len(authHeader) < 7 || !strings.EqualFold(authHeader[:7], "Bearer ") {
		return nil
	}
	token := strings.TrimSpace(authHeader[7:])
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil
	}

	secret := strings.TrimSpace(os.Getenv("JWT_SECRET"))
	if secret == "" {
		return nil
	}

	headerBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil
	}
	var hdr struct {
		Alg string `json:"alg"`
	}
	if err := json.Unmarshal(headerBytes, &hdr); err != nil || hdr.Alg != "HS256" {
		return nil
	}

	sigBytes, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(parts[0] + "." + parts[1]))
	if !hmac.Equal(sigBytes, mac.Sum(nil)) {
		return nil
	}

	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil
	}
	var claims map[string]interface{}
	if err := json.Unmarshal(payloadBytes, &claims); err != nil {
		return nil
	}
	return claims
}
