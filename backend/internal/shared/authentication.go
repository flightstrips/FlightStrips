package shared

import (
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type AuthenticatedUser struct {
	cid    string
	rating int
	token  *jwt.Token
	expiry int64
}

func (au *AuthenticatedUser) GetCid() string {
	return au.cid
}

func (au *AuthenticatedUser) GetRating() int {
	return au.rating
}

// NewAuthenticatedUser creates a new AuthenticatedUser and extracts expiry from the JWT token
func NewAuthenticatedUser(cid string, rating int, token *jwt.Token) AuthenticatedUser {
	var expiry int64

	if token != nil && token.Claims != nil {
		if claims, ok := token.Claims.(jwt.MapClaims); ok {
			if exp, exists := claims["exp"]; exists {
				if expFloat, ok := exp.(float64); ok {
					expiry = int64(expFloat)
				}
			}
		}
	}

	return AuthenticatedUser{
		cid:    cid,
		rating: rating,
		token:  token,
		expiry: expiry,
	}
}

// IsValid checks if the authenticated user is still valid based on expiry time
func (au *AuthenticatedUser) IsValid() bool {
	if au.expiry == 0 {
		return false
	}
	return time.Now().Unix() < au.expiry
}

type AuthenticationService interface {
	Validate(token string) (AuthenticatedUser, error)
}
