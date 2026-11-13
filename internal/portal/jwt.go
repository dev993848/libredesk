package portal

import (
	"errors"
	"fmt"

	"github.com/golang-jwt/jwt/v5"
)

// Claims holds the portal JWT payload. The format matches the widget identity
// JWT so customer backends can reuse the same signing code.
type Claims struct {
	ExternalUserID          string         `json:"external_user_id,omitempty"`
	Email                   string         `json:"email,omitempty"`
	FirstName               string         `json:"first_name,omitempty"`
	LastName                string         `json:"last_name,omitempty"`
	PhoneNumber             string         `json:"phone_number,omitempty"`
	PhoneNumberCountryCode  string         `json:"phone_number_country_code,omitempty"`
	ContactCustomAttributes map[string]any `json:"contact_custom_attributes,omitempty"`
	jwt.RegisteredClaims
}

// VerifyJWT verifies an HMAC-signed portal JWT with the given secret.
func VerifyJWT(tokenString, secret string) (Claims, error) {
	var claims Claims
	if secret == "" {
		return claims, errors.New("portal jwt secret is not configured")
	}
	token, err := jwt.ParseWithClaims(tokenString, &claims, func(token *jwt.Token) (any, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(secret), nil
	}, jwt.WithExpirationRequired())
	if err != nil {
		return Claims{}, err
	}
	if !token.Valid {
		return Claims{}, errors.New("invalid portal jwt")
	}
	return claims, nil
}
