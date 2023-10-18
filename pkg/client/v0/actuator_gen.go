// generated by 'threeport-codegen api-model' - do not edit

package v0

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
	"net/http"
)

// GetProfiles fetches all profiles.
// TODO: implement pagination
func GetProfiles(apiClient *http.Client, apiAddr string) (*[]v0.Profile, error) {
	var profiles []v0.Profile

	response, err := GetResponse(
		apiClient,
		fmt.Sprintf("%s/%s/profiles", apiAddr, ApiVersion),
		http.MethodGet,
		new(bytes.Buffer),
		map[string]string{},
		http.StatusOK,
	)
	if err != nil {
		return &profiles, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data)
	if err != nil {
		return &profiles, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&profiles); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	return &profiles, nil
}

// GetProfileByID fetches a profile by ID.
func GetProfileByID(apiClient *http.Client, apiAddr string, id uint) (*v0.Profile, error) {
	var profile v0.Profile

	response, err := GetResponse(
		apiClient,
		fmt.Sprintf("%s/%s/profiles/%d", apiAddr, ApiVersion, id),
		http.MethodGet,
		new(bytes.Buffer),
		map[string]string{},
		http.StatusOK,
	)
	if err != nil {
		return &profile, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data[0])
	if err != nil {
		return &profile, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&profile); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	return &profile, nil
}

// GetProfilesByQueryString fetches profiles by provided query string.
func GetProfilesByQueryString(apiClient *http.Client, apiAddr string, queryString string) (*[]v0.Profile, error) {
	var profiles []v0.Profile

	response, err := GetResponse(
		apiClient,
		fmt.Sprintf("%s/%s/profiles?%s", apiAddr, ApiVersion, queryString),
		http.MethodGet,
		new(bytes.Buffer),
		map[string]string{},
		http.StatusOK,
	)
	if err != nil {
		return &profiles, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data)
	if err != nil {
		return &profiles, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&profiles); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	return &profiles, nil
}

// GetProfileByName fetches a profile by name.
func GetProfileByName(apiClient *http.Client, apiAddr, name string) (*v0.Profile, error) {
	var profiles []v0.Profile

	response, err := GetResponse(
		apiClient,
		fmt.Sprintf("%s/%s/profiles?name=%s", apiAddr, ApiVersion, name),
		http.MethodGet,
		new(bytes.Buffer),
		map[string]string{},
		http.StatusOK,
	)
	if err != nil {
		return &v0.Profile{}, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data)
	if err != nil {
		return &v0.Profile{}, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&profiles); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	switch {
	case len(profiles) < 1:
		return &v0.Profile{}, errors.New(fmt.Sprintf("no profile with name %s", name))
	case len(profiles) > 1:
		return &v0.Profile{}, errors.New(fmt.Sprintf("more than one profile with name %s returned", name))
	}

	return &profiles[0], nil
}

// CreateProfile creates a new profile.
func CreateProfile(apiClient *http.Client, apiAddr string, profile *v0.Profile) (*v0.Profile, error) {
	ReplaceAssociatedObjectsWithNil(profile)
	jsonProfile, err := util.MarshalObject(profile)
	if err != nil {
		return profile, fmt.Errorf("failed to marshal provided object to JSON: %w", err)
	}

	response, err := GetResponse(
		apiClient,
		fmt.Sprintf("%s/%s/profiles", apiAddr, ApiVersion),
		http.MethodPost,
		bytes.NewBuffer(jsonProfile),
		map[string]string{},
		http.StatusCreated,
	)
	if err != nil {
		return profile, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data[0])
	if err != nil {
		return profile, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&profile); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	return profile, nil
}

// UpdateProfile updates a profile.
func UpdateProfile(apiClient *http.Client, apiAddr string, profile *v0.Profile) (*v0.Profile, error) {
	ReplaceAssociatedObjectsWithNil(profile)
	// capture the object ID, make a copy of the object, then remove fields that
	// cannot be updated in the API
	profileID := *profile.ID
	payloadProfile := *profile
	payloadProfile.ID = nil
	payloadProfile.CreatedAt = nil
	payloadProfile.UpdatedAt = nil

	jsonProfile, err := util.MarshalObject(payloadProfile)
	if err != nil {
		return profile, fmt.Errorf("failed to marshal provided object to JSON: %w", err)
	}

	response, err := GetResponse(
		apiClient,
		fmt.Sprintf("%s/%s/profiles/%d", apiAddr, ApiVersion, profileID),
		http.MethodPatch,
		bytes.NewBuffer(jsonProfile),
		map[string]string{},
		http.StatusOK,
	)
	if err != nil {
		return profile, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data[0])
	if err != nil {
		return profile, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&payloadProfile); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	payloadProfile.ID = &profileID
	return &payloadProfile, nil
}

// DeleteProfile deletes a profile by ID.
func DeleteProfile(apiClient *http.Client, apiAddr string, id uint) (*v0.Profile, error) {
	var profile v0.Profile

	response, err := GetResponse(
		apiClient,
		fmt.Sprintf("%s/%s/profiles/%d", apiAddr, ApiVersion, id),
		http.MethodDelete,
		new(bytes.Buffer),
		map[string]string{},
		http.StatusOK,
	)
	if err != nil {
		return &profile, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data[0])
	if err != nil {
		return &profile, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&profile); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	return &profile, nil
}

// GetTiers fetches all tiers.
// TODO: implement pagination
func GetTiers(apiClient *http.Client, apiAddr string) (*[]v0.Tier, error) {
	var tiers []v0.Tier

	response, err := GetResponse(
		apiClient,
		fmt.Sprintf("%s/%s/tiers", apiAddr, ApiVersion),
		http.MethodGet,
		new(bytes.Buffer),
		map[string]string{},
		http.StatusOK,
	)
	if err != nil {
		return &tiers, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data)
	if err != nil {
		return &tiers, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&tiers); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	return &tiers, nil
}

// GetTierByID fetches a tier by ID.
func GetTierByID(apiClient *http.Client, apiAddr string, id uint) (*v0.Tier, error) {
	var tier v0.Tier

	response, err := GetResponse(
		apiClient,
		fmt.Sprintf("%s/%s/tiers/%d", apiAddr, ApiVersion, id),
		http.MethodGet,
		new(bytes.Buffer),
		map[string]string{},
		http.StatusOK,
	)
	if err != nil {
		return &tier, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data[0])
	if err != nil {
		return &tier, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&tier); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	return &tier, nil
}

// GetTiersByQueryString fetches tiers by provided query string.
func GetTiersByQueryString(apiClient *http.Client, apiAddr string, queryString string) (*[]v0.Tier, error) {
	var tiers []v0.Tier

	response, err := GetResponse(
		apiClient,
		fmt.Sprintf("%s/%s/tiers?%s", apiAddr, ApiVersion, queryString),
		http.MethodGet,
		new(bytes.Buffer),
		map[string]string{},
		http.StatusOK,
	)
	if err != nil {
		return &tiers, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data)
	if err != nil {
		return &tiers, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&tiers); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	return &tiers, nil
}

// GetTierByName fetches a tier by name.
func GetTierByName(apiClient *http.Client, apiAddr, name string) (*v0.Tier, error) {
	var tiers []v0.Tier

	response, err := GetResponse(
		apiClient,
		fmt.Sprintf("%s/%s/tiers?name=%s", apiAddr, ApiVersion, name),
		http.MethodGet,
		new(bytes.Buffer),
		map[string]string{},
		http.StatusOK,
	)
	if err != nil {
		return &v0.Tier{}, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data)
	if err != nil {
		return &v0.Tier{}, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&tiers); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	switch {
	case len(tiers) < 1:
		return &v0.Tier{}, errors.New(fmt.Sprintf("no tier with name %s", name))
	case len(tiers) > 1:
		return &v0.Tier{}, errors.New(fmt.Sprintf("more than one tier with name %s returned", name))
	}

	return &tiers[0], nil
}

// CreateTier creates a new tier.
func CreateTier(apiClient *http.Client, apiAddr string, tier *v0.Tier) (*v0.Tier, error) {
	ReplaceAssociatedObjectsWithNil(tier)
	jsonTier, err := util.MarshalObject(tier)
	if err != nil {
		return tier, fmt.Errorf("failed to marshal provided object to JSON: %w", err)
	}

	response, err := GetResponse(
		apiClient,
		fmt.Sprintf("%s/%s/tiers", apiAddr, ApiVersion),
		http.MethodPost,
		bytes.NewBuffer(jsonTier),
		map[string]string{},
		http.StatusCreated,
	)
	if err != nil {
		return tier, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data[0])
	if err != nil {
		return tier, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&tier); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	return tier, nil
}

// UpdateTier updates a tier.
func UpdateTier(apiClient *http.Client, apiAddr string, tier *v0.Tier) (*v0.Tier, error) {
	ReplaceAssociatedObjectsWithNil(tier)
	// capture the object ID, make a copy of the object, then remove fields that
	// cannot be updated in the API
	tierID := *tier.ID
	payloadTier := *tier
	payloadTier.ID = nil
	payloadTier.CreatedAt = nil
	payloadTier.UpdatedAt = nil

	jsonTier, err := util.MarshalObject(payloadTier)
	if err != nil {
		return tier, fmt.Errorf("failed to marshal provided object to JSON: %w", err)
	}

	response, err := GetResponse(
		apiClient,
		fmt.Sprintf("%s/%s/tiers/%d", apiAddr, ApiVersion, tierID),
		http.MethodPatch,
		bytes.NewBuffer(jsonTier),
		map[string]string{},
		http.StatusOK,
	)
	if err != nil {
		return tier, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data[0])
	if err != nil {
		return tier, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&payloadTier); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	payloadTier.ID = &tierID
	return &payloadTier, nil
}

// DeleteTier deletes a tier by ID.
func DeleteTier(apiClient *http.Client, apiAddr string, id uint) (*v0.Tier, error) {
	var tier v0.Tier

	response, err := GetResponse(
		apiClient,
		fmt.Sprintf("%s/%s/tiers/%d", apiAddr, ApiVersion, id),
		http.MethodDelete,
		new(bytes.Buffer),
		map[string]string{},
		http.StatusOK,
	)
	if err != nil {
		return &tier, fmt.Errorf("call to threeport API returned unexpected response: %w", err)
	}

	jsonData, err := json.Marshal(response.Data[0])
	if err != nil {
		return &tier, fmt.Errorf("failed to marshal response data from threeport API: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.UseNumber()
	if err := decoder.Decode(&tier); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	return &tier, nil
}
