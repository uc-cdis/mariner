package mariner

import (
	"errors"
	"fmt"
	"net/http"
)

type TokenInfo struct {
	UserID string
}

func (server *Server) userID(r *http.Request) (userID string) {
	info, err := server.decodeToken(r.Header.Get(AUTH_HEADER))
	if err != nil {
		// log error
		fmt.Println("error decoding token: ", err)
	}
	return info.UserID
}

func (server *Server) decodeToken(token string) (*TokenInfo, error) {
	missingRequiredField := func(field string) error {
		msg := fmt.Sprintf(
			"failed to decode token: missing required field `%s`",
			field,
		)
		return errors.New(msg)
	}
	fieldTypeError := func(field string) error {
		msg := fmt.Sprintf(
			"failed to decode token: field `%s` has wrong type",
			field,
		)
		return errors.New(msg)
	}
	// server.logger.Debug("decoding token: %s", token)

	claims, err := server.jwtApp.Decode(token)
	if err != nil {
		fmt.Println("error decoding token: ", err)
	}
	contextInterface, exists := (*claims)["context"]
	if !exists {
		return nil, missingRequiredField("context")
	}
	context, casted := contextInterface.(map[string]interface{})
	if !casted {
		return nil, fieldTypeError("context")
	}
	userInterface, exists := context["user"]
	if !exists {
		return nil, missingRequiredField("user")
	}
	user, casted := userInterface.(map[string]interface{})
	if !casted {
		return nil, fieldTypeError("user")
	}
	usernameInterface, exists := user["name"]
	if !exists {
		return nil, missingRequiredField("name")
	}
	username, casted := usernameInterface.(string)
	if !casted {
		return nil, fieldTypeError("name")
	}
	info := TokenInfo{
		UserID: username,
	}
	fmt.Println("here is token info:")
	printJSON(info)
	return &info, nil
}
