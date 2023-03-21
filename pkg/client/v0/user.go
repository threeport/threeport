package client

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	v0 "github.com/threeport/threeport/pkg/api/v0"
)

// CreateUser creates a new user in the Threeport API from a json object that
// contains the user attributes.
func CreateUser(jsonUser []byte, apiAddr, apiToken string) (*v0.User, error) {
	var user v0.User

	response, err := GetResponse(fmt.Sprintf("%s/%s/users", apiAddr, ApiVersion), apiToken, http.MethodPost, bytes.NewBuffer(jsonUser), http.StatusCreated)
	if err != nil {
		return &v0.User{}, err
	}

	jsonData, err := json.Marshal(response.Data[0])
	if err != nil {
		return &v0.User{}, err
	}

	err = json.Unmarshal(jsonData, &user)
	if err != nil {
		return &v0.User{}, err
	}

	return &user, nil
}

// GetUser fetches a user from the Threeport API by email address.
func GetUser(email, apiAddr, apiToken string) (*v0.User, error) {
	var users []v0.User

	response, err := GetResponse(fmt.Sprintf("%s/%s/users?email=%s", apiAddr, ApiVersion, email), apiToken, http.MethodGet, new(bytes.Buffer), http.StatusOK)
	if err != nil {
		return &v0.User{}, err
	}
	jsonData, err := json.Marshal(response.Data)
	if err != nil {
		return &v0.User{}, err
	}

	err = json.Unmarshal(jsonData, &users)
	if err != nil {
		return &v0.User{}, err
	}

	switch {
	case len(users) < 1:
		return &v0.User{}, errors.New(fmt.Sprintf("no users with email %s", email))
	case len(users) > 1:
		return &v0.User{}, errors.New(fmt.Sprintf("more than one user with email %s returned", email))
	}

	return &users[0], nil
}

// GetUserById fetches a user from the Threeport API by id.
func GetUserById(id uint, apiAddr, apiToken string) (*v0.User, error) {
	var user v0.User

	response, err := GetResponse(fmt.Sprintf("%s/%s/users/%d", apiAddr, ApiVersion, id), apiToken, http.MethodGet, new(bytes.Buffer), http.StatusOK)
	if err != nil {
		return &v0.User{}, err
	}

	jsonData, err := json.Marshal(response.Data[0])
	if err != nil {
		return &v0.User{}, err
	}

	err = json.Unmarshal(jsonData, &user)
	if err != nil {
		return &v0.User{}, err
	}

	return &user, nil
}
