package auth

import (
	"errors"
	"fmt"
	"os"

	"github.com/golang-jwt/jwt/v5"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/router"
)

var sharedSecret []byte

func Init() {
	sharedSecret = []byte(os.Getenv("shared_secret"))
}

func validateUserJWT(tokenString string) (*UserClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &UserClaims{}, func(token *jwt.Token) (any, error) {
		return sharedSecret, nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Name}))
	if err != nil {
		return nil, err
	}

	if claims, ok := token.Claims.(*UserClaims); ok && token.Valid {
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
		app := e.App.(*pocketbase.PocketBase)
		// check if user exists in the database
		fmt.Println("userClaims.Email", userClaims.Email)
		userRecord, err := app.FindFirstRecordByData("users", "email", userClaims.Email)
		if err != nil {
			return e.BadRequestError("Users is not found, please have the admin create a listpocket user first.", err)
		}
		return apis.RecordAuthResponse(e, userRecord, "listpocket-token-exchange", nil)
	})
}
