package jwt

import (
	"fmt"
	"time"

	"github.com/dgrijalva/jwt-go"
)

type CustomClaims struct {
	leeway   int64
	stdClaim jwt.StandardClaims
}

func NewCustomClaims(leeway int64) *CustomClaims {
	return &CustomClaims{leeway: leeway, stdClaim: jwt.StandardClaims{}}
}

// There is no accounting for clock skew.
// As well, if any of the above claims are not in the token, it will still
// be considered a valid claim.
func (c *CustomClaims) Valid() error {
	vErr := new(jwt.ValidationError)
	now := jwt.TimeFunc().Unix() - c.leeway

	if c.stdClaim.VerifyExpiresAt(now, true) == false {
		delta := time.Unix(now, 0).Sub(time.Unix(c.stdClaim.ExpiresAt, 0))
		vErr.Inner = fmt.Errorf("token is expired by %v", delta)
		vErr.Errors |= jwt.ValidationErrorExpired
	}

	if c.stdClaim.VerifyIssuedAt(now, false) == false {
		vErr.Inner = fmt.Errorf("Token used before issued")
		vErr.Errors |= jwt.ValidationErrorIssuedAt
	}

	if c.stdClaim.VerifyNotBefore(now, false) == false {
		vErr.Inner = fmt.Errorf("token is not valid yet")
		vErr.Errors |= jwt.ValidationErrorNotValidYet
	}

	if vErr.Errors == 0 {
		return nil
	}

	return vErr
}
