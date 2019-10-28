package mariner

import (
	"errors"
	"fmt"
)

// this is verbatim arborist source code .. feels stupid to duplicate code but not sure how else to handle the token properly - to extract the userID

type TokenInfo struct {
	UserID string
}

func (server *Server) userID(token string) (userID string) {
	aud := []string{"openid"}
	info, err := server.decodeToken(token, aud)
	if err != nil {
		// log error
		fmt.Println("error decoding token: ", err)
	}
	return info.UserID
}

func (server *Server) decodeToken(token string, aud []string) (*TokenInfo, error) {
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
	/* Commented out for debugging
	claims, err := server.jwtApp.Decode(token)
	if err != nil {
		return nil, fmt.Errorf("error decoding token: %s", err.Error())
	}
	expected := &authutils.Expected{Audiences: aud}
	err = expected.Validate(claims)
	if err != nil {
		return nil, fmt.Errorf("error decoding token: %s", err.Error())
	}
	*/
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
