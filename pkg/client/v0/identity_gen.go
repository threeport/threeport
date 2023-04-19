// generated by 'threeport-codegen api-model' - do not edit

package v0

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	client "github.com/threeport/threeport/pkg/client"
	"net/http"
)

// GetUsers fetches all users.
// TODO: implement pagination
func GetUsers(apiAddr, apiToken string) (*[]v0.User, error) {
	var users []v0.User

	response, err := GetResponse(
		fmt.Sprintf("%s/%s/users", apiAddr, ApiVersion),
		apiToken,
		http.MethodGet,
		new(bytes.Buffer),
		http.StatusOK,
	)
	if err != nil {
		return &users, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data)
	if err != nil {
		return &users, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&users); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	return &users, nil
}

// GetUserByID fetches a user by ID.
func GetUserByID(id uint, apiAddr, apiToken string) (*v0.User, error) {
	var user v0.User

	response, err := GetResponse(
		fmt.Sprintf("%s/%s/users/%d", apiAddr, ApiVersion, id),
		apiToken,
		http.MethodGet,
		new(bytes.Buffer),
		http.StatusOK,
	)
	if err != nil {
		return &user, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data[0])
	if err != nil {
		return &user, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&user); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	return &user, nil
}

// GetUserByName fetches a user by name.
func GetUserByName(name, apiAddr, apiToken string) (*v0.User, error) {
	var users []v0.User

	response, err := GetResponse(
		fmt.Sprintf("%s/%s/users?name=%s", apiAddr, ApiVersion, name),
		apiToken,
		http.MethodGet,
		new(bytes.Buffer),
		http.StatusOK,
	)
	if err != nil {
		return &v0.User{}, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data)
	if err != nil {
		return &v0.User{}, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&users); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	switch {
	case len(users) < 1:
		return &v0.User{}, errors.New(fmt.Sprintf("no workload definitions with name %s", name))
	case len(users) > 1:
		return &v0.User{}, errors.New(fmt.Sprintf("more than one workload definition with name %s returned", name))
	}

	return &users[0], nil
}

// CreateUser creates a new user.
func CreateUser(user *v0.User, apiAddr, apiToken string) (*v0.User, error) {
	jsonUser, err := client.MarshalObject(user)
	if err != nil {
		return user, fmt.Errorf("failed to marshal provided object to JSON: %w", err)
	}

	response, err := GetResponse(
		fmt.Sprintf("%s/%s/users", apiAddr, ApiVersion),
		apiToken,
		http.MethodPost,
		bytes.NewBuffer(jsonUser),
		http.StatusCreated,
	)
	if err != nil {
		return user, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data[0])
	if err != nil {
		return user, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&user); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	return user, nil
}

// UpdateUser updates a user.
func UpdateUser(user *v0.User, apiAddr, apiToken string) (*v0.User, error) {
	// capture the object ID then remove it from the object since the API will not
	// allow an update the ID field
	userID := *user.ID
	user.ID = nil

	jsonUser, err := client.MarshalObject(user)
	if err != nil {
		return user, fmt.Errorf("failed to marshal provided object to JSON: %w", err)
	}

	response, err := GetResponse(
		fmt.Sprintf("%s/%s/users/%d", apiAddr, ApiVersion, userID),
		apiToken,
		http.MethodPatch,
		bytes.NewBuffer(jsonUser),
		http.StatusOK,
	)
	if err != nil {
		return user, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data[0])
	if err != nil {
		return user, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&user); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	return user, nil
}

// DeleteUser deletes a user by ID.
func DeleteUser(id uint, apiAddr, apiToken string) (*v0.User, error) {
	var user v0.User

	response, err := GetResponse(
		fmt.Sprintf("%s/%s/users/%d", apiAddr, ApiVersion, id),
		apiToken,
		http.MethodDelete,
		new(bytes.Buffer),
		http.StatusOK,
	)
	if err != nil {
		return &user, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data[0])
	if err != nil {
		return &user, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&user); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	return &user, nil
}

// GetCompanies fetches all companies.
// TODO: implement pagination
func GetCompanies(apiAddr, apiToken string) (*[]v0.Company, error) {
	var companies []v0.Company

	response, err := GetResponse(
		fmt.Sprintf("%s/%s/companies", apiAddr, ApiVersion),
		apiToken,
		http.MethodGet,
		new(bytes.Buffer),
		http.StatusOK,
	)
	if err != nil {
		return &companies, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data)
	if err != nil {
		return &companies, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&companies); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	return &companies, nil
}

// GetCompanyByID fetches a company by ID.
func GetCompanyByID(id uint, apiAddr, apiToken string) (*v0.Company, error) {
	var company v0.Company

	response, err := GetResponse(
		fmt.Sprintf("%s/%s/companies/%d", apiAddr, ApiVersion, id),
		apiToken,
		http.MethodGet,
		new(bytes.Buffer),
		http.StatusOK,
	)
	if err != nil {
		return &company, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data[0])
	if err != nil {
		return &company, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&company); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	return &company, nil
}

// GetCompanyByName fetches a company by name.
func GetCompanyByName(name, apiAddr, apiToken string) (*v0.Company, error) {
	var companies []v0.Company

	response, err := GetResponse(
		fmt.Sprintf("%s/%s/companies?name=%s", apiAddr, ApiVersion, name),
		apiToken,
		http.MethodGet,
		new(bytes.Buffer),
		http.StatusOK,
	)
	if err != nil {
		return &v0.Company{}, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data)
	if err != nil {
		return &v0.Company{}, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&companies); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	switch {
	case len(companies) < 1:
		return &v0.Company{}, errors.New(fmt.Sprintf("no workload definitions with name %s", name))
	case len(companies) > 1:
		return &v0.Company{}, errors.New(fmt.Sprintf("more than one workload definition with name %s returned", name))
	}

	return &companies[0], nil
}

// CreateCompany creates a new company.
func CreateCompany(company *v0.Company, apiAddr, apiToken string) (*v0.Company, error) {
	jsonCompany, err := client.MarshalObject(company)
	if err != nil {
		return company, fmt.Errorf("failed to marshal provided object to JSON: %w", err)
	}

	response, err := GetResponse(
		fmt.Sprintf("%s/%s/companies", apiAddr, ApiVersion),
		apiToken,
		http.MethodPost,
		bytes.NewBuffer(jsonCompany),
		http.StatusCreated,
	)
	if err != nil {
		return company, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data[0])
	if err != nil {
		return company, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&company); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	return company, nil
}

// UpdateCompany updates a company.
func UpdateCompany(company *v0.Company, apiAddr, apiToken string) (*v0.Company, error) {
	// capture the object ID then remove it from the object since the API will not
	// allow an update the ID field
	companyID := *company.ID
	company.ID = nil

	jsonCompany, err := client.MarshalObject(company)
	if err != nil {
		return company, fmt.Errorf("failed to marshal provided object to JSON: %w", err)
	}

	response, err := GetResponse(
		fmt.Sprintf("%s/%s/companies/%d", apiAddr, ApiVersion, companyID),
		apiToken,
		http.MethodPatch,
		bytes.NewBuffer(jsonCompany),
		http.StatusOK,
	)
	if err != nil {
		return company, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data[0])
	if err != nil {
		return company, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&company); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	return company, nil
}

// DeleteCompany deletes a company by ID.
func DeleteCompany(id uint, apiAddr, apiToken string) (*v0.Company, error) {
	var company v0.Company

	response, err := GetResponse(
		fmt.Sprintf("%s/%s/companies/%d", apiAddr, ApiVersion, id),
		apiToken,
		http.MethodDelete,
		new(bytes.Buffer),
		http.StatusOK,
	)
	if err != nil {
		return &company, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data[0])
	if err != nil {
		return &company, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&company); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	return &company, nil
}
