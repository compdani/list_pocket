package auth

import (
	"errors"
	"os"
	"strings"

	"ganigeorgiev.com/pocketbase/pocketbase/apis"
	"ganigeorgiev.com/pocketbase/pocketbase/core"
	"ganigeorgiev.com/pocketbase/pocketbase/tools/router"
	"github.com/golang-jwt/jwt/v5"
)

var sharedSecret []byte

func Init() {
	sharedSecret = []byte(os.Getenv("shared_secret"))
}

func validateUserJWT(tokenString string) (*UserClaims, error) {
	if strings.TrimSpace(string(sharedSecret)) == "" {
		return nil, errors.New("token exchange is not configured")
	}
	token, err := jwt.ParseWithClaims(tokenString, &UserClaims{}, func(token *jwt.Token) (any, error) {
		return sharedSecret, nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Name}), jwt.WithExpirationRequired())
	if err != nil {
		return nil, err
	}

	if claims, ok := token.Claims.(*UserClaims); ok && token.Valid && strings.TrimSpace(claims.Email) != "" {
		return claims, nil
	}

	return nil, errors.New("invalid token")
}

func RegisterExchangeRoutes(c *router.Router[*core.RequestEvent]) {
	Init()
	c.GET("/user_token", func(e *core.RequestEvent) error {
		token := e.Request.Header.Get("ExchangeToken")
		if token == "" {
			return e.BadRequestError("No token provided", nil)
		}
		userClaims, err := validateUserJWT(token)
		if err != nil {
			return e.BadRequestError("Invalid token", err)
		}
		app := e.App
		// check if user exists in the database
		userRecord, err := app.FindFirstRecordByData("users", "email", userClaims.Email)
		if err != nil {
			return e.BadRequestError("Users is not found, please have the admin create a listpocket user first.", err)
		}
		if userRecord.GetString("status") != UserStatusEnabled || userRecord.GetString("user_type") != UserTypeUser || userRecord.GetString("twofa_type") != "" {
			return e.ForbiddenError("Use the application login flow.", nil)
		}
		return apis.RecordAuthResponse(e, userRecord, "listpocket-token-exchange", nil)
	})
}
