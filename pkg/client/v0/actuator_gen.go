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

// GetProfiles fetches all profiles.
// TODO: implement pagination
func GetProfiles(apiAddr, apiToken string) (*[]v0.Profile, error) {
	var profiles []v0.Profile

	response, err := GetResponse(
		fmt.Sprintf("%s/%s/profiles", apiAddr, ApiVersion),
		apiToken,
		http.MethodGet,
		new(bytes.Buffer),
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
func GetProfileByID(id uint, apiAddr, apiToken string) (*v0.Profile, error) {
	var profile v0.Profile

	response, err := GetResponse(
		fmt.Sprintf("%s/%s/profiles/%d", apiAddr, ApiVersion, id),
		apiToken,
		http.MethodGet,
		new(bytes.Buffer),
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

// GetProfileByName fetches a profile by name.
func GetProfileByName(name, apiAddr, apiToken string) (*v0.Profile, error) {
	var profiles []v0.Profile

	response, err := GetResponse(
		fmt.Sprintf("%s/%s/profiles?name=%s", apiAddr, ApiVersion, name),
		apiToken,
		http.MethodGet,
		new(bytes.Buffer),
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
		return &v0.Profile{}, errors.New(fmt.Sprintf("no workload definitions with name %s", name))
	case len(profiles) > 1:
		return &v0.Profile{}, errors.New(fmt.Sprintf("more than one workload definition with name %s returned", name))
	}

	return &profiles[0], nil
}

// CreateProfile creates a new profile.
func CreateProfile(profile *v0.Profile, apiAddr, apiToken string) (*v0.Profile, error) {
	jsonProfile, err := client.MarshalObject(profile)
	if err != nil {
		return profile, fmt.Errorf("failed to marshal provided object to JSON: %w", err)
	}

	response, err := GetResponse(
		fmt.Sprintf("%s/%s/profiles", apiAddr, ApiVersion),
		apiToken,
		http.MethodPost,
		bytes.NewBuffer(jsonProfile),
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
func UpdateProfile(profile *v0.Profile, apiAddr, apiToken string) (*v0.Profile, error) {
	// capture the object ID then remove it from the object since the API will not
	// allow an update the ID field
	profileID := *profile.ID
	profile.ID = nil

	jsonProfile, err := client.MarshalObject(profile)
	if err != nil {
		return profile, fmt.Errorf("failed to marshal provided object to JSON: %w", err)
	}

	response, err := GetResponse(
		fmt.Sprintf("%s/%s/profiles/%d", apiAddr, ApiVersion, profileID),
		apiToken,
		http.MethodPatch,
		bytes.NewBuffer(jsonProfile),
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
	if err := decoder.Decode(&profile); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	return profile, nil
}

// DeleteProfile deletes a profile by ID.
func DeleteProfile(id uint, apiAddr, apiToken string) (*v0.Profile, error) {
	var profile v0.Profile

	response, err := GetResponse(
		fmt.Sprintf("%s/%s/profiles/%d", apiAddr, ApiVersion, id),
		apiToken,
		http.MethodDelete,
		new(bytes.Buffer),
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
func GetTiers(apiAddr, apiToken string) (*[]v0.Tier, error) {
	var tiers []v0.Tier

	response, err := GetResponse(
		fmt.Sprintf("%s/%s/tiers", apiAddr, ApiVersion),
		apiToken,
		http.MethodGet,
		new(bytes.Buffer),
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
func GetTierByID(id uint, apiAddr, apiToken string) (*v0.Tier, error) {
	var tier v0.Tier

	response, err := GetResponse(
		fmt.Sprintf("%s/%s/tiers/%d", apiAddr, ApiVersion, id),
		apiToken,
		http.MethodGet,
		new(bytes.Buffer),
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

// GetTierByName fetches a tier by name.
func GetTierByName(name, apiAddr, apiToken string) (*v0.Tier, error) {
	var tiers []v0.Tier

	response, err := GetResponse(
		fmt.Sprintf("%s/%s/tiers?name=%s", apiAddr, ApiVersion, name),
		apiToken,
		http.MethodGet,
		new(bytes.Buffer),
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
		return &v0.Tier{}, errors.New(fmt.Sprintf("no workload definitions with name %s", name))
	case len(tiers) > 1:
		return &v0.Tier{}, errors.New(fmt.Sprintf("more than one workload definition with name %s returned", name))
	}

	return &tiers[0], nil
}

// CreateTier creates a new tier.
func CreateTier(tier *v0.Tier, apiAddr, apiToken string) (*v0.Tier, error) {
	jsonTier, err := client.MarshalObject(tier)
	if err != nil {
		return tier, fmt.Errorf("failed to marshal provided object to JSON: %w", err)
	}

	response, err := GetResponse(
		fmt.Sprintf("%s/%s/tiers", apiAddr, ApiVersion),
		apiToken,
		http.MethodPost,
		bytes.NewBuffer(jsonTier),
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
func UpdateTier(tier *v0.Tier, apiAddr, apiToken string) (*v0.Tier, error) {
	// capture the object ID then remove it from the object since the API will not
	// allow an update the ID field
	tierID := *tier.ID
	tier.ID = nil

	jsonTier, err := client.MarshalObject(tier)
	if err != nil {
		return tier, fmt.Errorf("failed to marshal provided object to JSON: %w", err)
	}

	response, err := GetResponse(
		fmt.Sprintf("%s/%s/tiers/%d", apiAddr, ApiVersion, tierID),
		apiToken,
		http.MethodPatch,
		bytes.NewBuffer(jsonTier),
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
	if err := decoder.Decode(&tier); err != nil {
		return nil, fmt.Errorf("failed to decode object in response data from threeport API: %w", err)
	}

	return tier, nil
}

// DeleteTier deletes a tier by ID.
func DeleteTier(id uint, apiAddr, apiToken string) (*v0.Tier, error) {
	var tier v0.Tier

	response, err := GetResponse(
		fmt.Sprintf("%s/%s/tiers/%d", apiAddr, ApiVersion, id),
		apiToken,
		http.MethodDelete,
		new(bytes.Buffer),
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
