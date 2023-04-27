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

// GetUsers feteches all users.
// TODO: implement pagination
func GetUsers(httpsClient *http.Client, apiAddr string) (*[]v0.User, error) {
	var users []v0.User

	response, err := GetResponse(
		httpsClient,
		fmt.Sprintf("%s/%s/users", apiAddr, ApiVersion),
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

// GetUserByID feteches a user by ID.
func GetUserByID(httpsClient *http.Client, id uint, apiAddr string) (*v0.User, error) {
	var user v0.User

	response, err := GetResponse(
		httpsClient,
		fmt.Sprintf("%s/%s/users/%d", apiAddr, ApiVersion, id),
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

// GetUserByName feteches a user by name.
func GetUserByName(httpsClient *http.Client, name, apiAddr string) (*v0.User, error) {
	var users []v0.User

	response, err := GetResponse(
		httpsClient,
		fmt.Sprintf("%s/%s/users?name=%s", apiAddr, ApiVersion, name),
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
func CreateUser(httpsClient *http.Client, user *v0.User, apiAddr string) (*v0.User, error) {
	jsonUser, err := client.MarshalObject(user)
	if err != nil {
		return user, fmt.Errorf("failed to marshal provided object to JSON: %w", err)
	}

	response, err := GetResponse(
		httpsClient,
		fmt.Sprintf("%s/%s/users", apiAddr, ApiVersion),
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
func UpdateUser(httpsClient *http.Client, user *v0.User, apiAddr string) (*v0.User, error) {
	// capture the object ID then remove it from the object since the API will not
	// allow an update the ID field
	userID := *user.ID
	user.ID = nil

	jsonUser, err := client.MarshalObject(user)
	if err != nil {
		return user, fmt.Errorf("failed to marshal provided object to JSON: %w", err)
	}

	response, err := GetResponse(
		httpsClient,
		fmt.Sprintf("%s/%s/users/%d", apiAddr, ApiVersion, userID),
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
func DeleteUser(httpsClient *http.Client, id uint, apiAddr string) (*v0.User, error) {
	var user v0.User

	response, err := GetResponse(
		httpsClient,
		fmt.Sprintf("%s/%s/users/%d", apiAddr, ApiVersion, id),
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

// GetCompanies feteches all companies.
// TODO: implement pagination
func GetCompanies(httpsClient *http.Client, apiAddr string) (*[]v0.Company, error) {
	var companies []v0.Company

	response, err := GetResponse(
		httpsClient,
		fmt.Sprintf("%s/%s/companies", apiAddr, ApiVersion),
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

// GetCompanyByID feteches a company by ID.
func GetCompanyByID(httpsClient *http.Client, id uint, apiAddr string) (*v0.Company, error) {
	var company v0.Company

	response, err := GetResponse(
		httpsClient,
		fmt.Sprintf("%s/%s/companies/%d", apiAddr, ApiVersion, id),
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

// GetCompanyByName feteches a company by name.
func GetCompanyByName(httpsClient *http.Client, name, apiAddr string) (*v0.Company, error) {
	var companies []v0.Company

	response, err := GetResponse(
		httpsClient,
		fmt.Sprintf("%s/%s/companies?name=%s", apiAddr, ApiVersion, name),
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
func CreateCompany(httpsClient *http.Client, company *v0.Company, apiAddr string) (*v0.Company, error) {
	jsonCompany, err := client.MarshalObject(company)
	if err != nil {
		return company, fmt.Errorf("failed to marshal provided object to JSON: %w", err)
	}

	response, err := GetResponse(
		httpsClient,
		fmt.Sprintf("%s/%s/companies", apiAddr, ApiVersion),
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
func UpdateCompany(httpsClient *http.Client, company *v0.Company, apiAddr string) (*v0.Company, error) {
	// capture the object ID then remove it from the object since the API will not
	// allow an update the ID field
	companyID := *company.ID
	company.ID = nil

	jsonCompany, err := client.MarshalObject(company)
	if err != nil {
		return company, fmt.Errorf("failed to marshal provided object to JSON: %w", err)
	}

	response, err := GetResponse(
		httpsClient,
		fmt.Sprintf("%s/%s/companies/%d", apiAddr, ApiVersion, companyID),
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
func DeleteCompany(httpsClient *http.Client, id uint, apiAddr string) (*v0.Company, error) {
	var company v0.Company

	response, err := GetResponse(
		httpsClient,
		fmt.Sprintf("%s/%s/companies/%d", apiAddr, ApiVersion, id),
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
