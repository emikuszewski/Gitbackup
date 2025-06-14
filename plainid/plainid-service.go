package plainid

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/plainid/git-backup/config"
	"github.com/rs/zerolog/log"
	"golang.org/x/oauth2/clientcredentials"
)

type Policy struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	State      string `json:"state"`
	AccessType string `json:"accessType"`
}

type Meta struct {
	Total  int `json:"total"`
	Limit  int `json:"limit"`
	Offset int `json:"offset"`
}

type PolicyResponse struct {
	Data []Policy `json:"data"`
	Meta Meta     `json:"meta"`
}

type Application struct {
	WSID             string   `json:"-"` // Use `-` to omit this field when marshaling to JSON
	ID               string   `json:"applicationId"`
	Name             string   `json:"displayName"`
	Description      string   `json:"description"`
	LogoURL          string   `json:"logoUrl"`
	ColorIndication  string   `json:"colorIndication"`
	AssetTemplateIDs []string `json:"assetTemplateIds"`
}

type Environment struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type Workspace struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type Identity struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func (s Application) AsJSON() (string, error) {
	b, err := json.Marshal(s)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

type Service struct {
	cfg    config.Config
	client *http.Client
}

func NewService(cfg config.Config) *Service {
	oauth2Config := clientcredentials.Config{
		ClientID:     cfg.PlainID.ClientID,
		ClientSecret: cfg.PlainID.ClientSecret,
		TokenURL:     fmt.Sprintf("%s/api/1.0/api-key/token", cfg.PlainID.BaseURL),
	}

	client := oauth2Config.Client(context.Background())

	return &Service{
		cfg:    cfg,
		client: client,
	}
}

func (s Service) Environments() ([]Environment, error) {
	baseURL := fmt.Sprintf("%s/env-mgmt/environment", s.cfg.PlainID.BaseURL)
	log.Debug().Msgf("Fetching environments from PlainID %s...", baseURL)

	req, err := http.NewRequest("GET", baseURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get environments for whildcard:  %s %s", resp.Status, body)
	}

	type EnvsResponse struct {
		Data []Environment `json:"data"`
		Meta Meta          `json:"meta"`
	}

	var envsResp EnvsResponse
	err = json.Unmarshal(body, &envsResp)
	if err != nil {
		return nil, fmt.Errorf("failed to parse environments response: %w", err)
	}

	return envsResp.Data, nil
}

func (s Service) Workspaces(envID string) ([]Workspace, error) {
	baseURL := fmt.Sprintf("%s/env-mgmt/1.0-int.1/authorization-workspaces/%s?offset=0&limit=100", s.cfg.PlainID.BaseURL, envID)

	req, err := http.NewRequest("GET", baseURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get environments for whildcard:  %s %s", resp.Status, body)
	}

	type WSsResponse struct {
		Data []Workspace `json:"data"`
		Meta Meta        `json:"meta"`
	}

	var wssResp WSsResponse
	err = json.Unmarshal(body, &wssResp)
	if err != nil {
		return nil, fmt.Errorf("failed to parse workspaces response: %w", err)
	}

	return wssResp.Data, nil
}

func (s Service) Identities(envID string) ([]Identity, error) {
	baseURL := fmt.Sprintf("%s/env-mgmt/1.0/identity-workspaces/%s?offset=0&limit=100", s.cfg.PlainID.BaseURL, envID)

	req, err := http.NewRequest("GET", baseURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get environments for whildcard:  %s %s", resp.Status, body)
	}

	type IdentitiesResponse struct {
		Data []Identity `json:"data"`
	}

	var identitiesResp IdentitiesResponse
	err = json.Unmarshal(body, &identitiesResp)
	if err != nil {
		return nil, fmt.Errorf("failed to parse workspaces response: %w", err)
	}

	return identitiesResp.Data, nil
}

func (s Service) Applications(envID, wsID string) ([]Application, error) {
	type AppInfo struct {
		ID   string `json:"id"`
		Name string `json:"name"`
		WSID string `json:"authWsId"`
	}

	type AppInfoResponse struct {
		Data   []AppInfo `json:"data"`
		Total  int       `json:"total"`
		Limit  int       `json:"limit"`
		Offset int       `json:"offset"`
	}

	limit := 50
	offset := 0
	var appInfos []AppInfo

	for {
		uRL := fmt.Sprintf("%s/policy-mgmt/1.0/applications/%s?detailed=true&limit=%d&offset=%d",
			s.cfg.PlainID.BaseURL,
			envID,
			limit,
			offset)

		req, err := http.NewRequest("GET", uRL, nil)
		if err != nil {
			return nil, err
		}

		resp, err := s.client.Do(req)
		if err != nil {
			return nil, err
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()

		if err != nil {
			return nil, err
		}

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("failed to download apps for %s: %s %s", wsID, resp.Status, body)
		}

		var appResp AppInfoResponse
		err = json.Unmarshal(body, &appResp)
		if err != nil {
			return nil, fmt.Errorf("failed to parse applications response: %w", err)
		}

		// Append apps only from specific workspace
		for _, app := range appResp.Data {
			if app.WSID == wsID {
				appInfos = append(appInfos, app)
			}
		}

		// Check if we've retrieved all apps
		if len(appResp.Data) < limit || offset+len(appResp.Data) >= appResp.Total {
			break
		}
		// Move to the next page
		offset += limit
	}

	// export applications
	apps := make([]Application, 0, len(appInfos))
	for _, appInfo := range appInfos {
		baseURL := fmt.Sprintf("%s/api/1.0/applications/%s/%s", s.cfg.PlainID.BaseURL, envID, appInfo.ID)

		req, err := http.NewRequest("GET", baseURL, nil)
		if err != nil {
			return nil, err
		}

		resp, err := s.client.Do(req)
		if err != nil {
			return nil, err
		}

		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("failed to download application for %s: %s %s", appInfo.ID, resp.Status, body)
		}

		type AppResponse struct {
			Data Application `json:"data"`
		}

		var appResponse AppResponse
		err = json.Unmarshal(body, &appResponse)
		if err != nil {
			return nil, fmt.Errorf("failed to parse applications response: %w", err)
		}

		app := appResponse.Data
		app.WSID = appInfo.WSID
		apps = append(apps, app)
	}

	return apps, nil
}

// returns App policies
func (s Service) AppPolicies(envID, wsID, appID string) ([]string, error) {
	//todo at the moment we support up to 1000 policies per app which should be enough
	baseURL := fmt.Sprintf("%s/policy-mgmt/1.0/policies/%s?%s=%s", s.cfg.PlainID.BaseURL, envID,
		url.QueryEscape("filter[appId]"), appID)

	req, err := http.NewRequest("GET", baseURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to download policies for %s: %s %s", appID, resp.Status, body)
	}

	var pols PolicyResponse
	err = json.Unmarshal(body, &pols)

	// retrieve policies now
	policies := make([]string, 0)
	for _, pol := range pols.Data {
		if pol.State == "Inactive" {
			continue
		}
		baseURL = fmt.Sprintf("%s/api/2.0/policies/%s?%s=%s&%s=%s&extendedSchema=true", s.cfg.PlainID.BaseURL, envID,
			url.QueryEscape("filter[authWsId]"), wsID, url.QueryEscape("filter[id]"), pol.ID)
		req, err = http.NewRequest("GET", baseURL, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to download policy for %s: %w", err)
		}
		req.Header.Set("Accept", "text/plain;language=rego")

		resp, err := s.client.Do(req)
		if err != nil {
			return nil, err
		}

		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("failed to download policy for %s: %s %s", wsID, resp.Status, body)
		}

		policies = append(policies, string(body))
	}
	return policies, err
}

func (s Service) AppAPIMapper(envID, appID string) (string, error) {
	baseURL := fmt.Sprintf("%s/api/1.0/api-mapper-sets/%s/%s", s.cfg.PlainID.BaseURL, envID, appID)

	req, err := http.NewRequest("GET", baseURL, nil)
	if err != nil {
		return "", err
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to download api mapper for %s: %s %s", appID, resp.Status, body)
	}

	return string(body), nil
}

func (s Service) AssetTemplateIDs(wsID string) ([]string, error) {
	baseURL := fmt.Sprintf("%s/internal-assets/4.0/asset-types?offset=0&limit=50&%s=%s", s.cfg.PlainID.BaseURL, url.QueryEscape("filter[ownerId]"), wsID)

	req, err := http.NewRequest("GET", baseURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch asset templates for %s: %s %s", wsID, resp.Status, body)
	}

	type AssetTemplatesResp struct {
		Data []struct {
			ID          string `json:"id"`
			ExtID       string `json:"externalId"`
			Name        string `json:"name"`
			Description string `json:"description"`
			OwnerID     string `json:"ownerId"`
		}
	}

	var appResponse AssetTemplatesResp
	err = json.Unmarshal(body, &appResponse)
	if err != nil {
		return nil, fmt.Errorf("failed to parse applications response: %w", err)
	}

	var assetTemplateIDs []string
	for _, app := range appResponse.Data {
		assetTemplateIDs = append(assetTemplateIDs, app.ExtID)
	}
	return assetTemplateIDs, nil
}

func (s Service) AssetTemplate(envID, assetTemplateID string) (string, error) {
	baseURL := fmt.Sprintf("%s/api/1.0/asset-templates/%s/%s", s.cfg.PlainID.BaseURL, envID, assetTemplateID)

	req, err := http.NewRequest("GET", baseURL, nil)
	if err != nil {
		return "", err
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to download asset template ID for %s: %s %s", assetTemplateID, resp.Status, body)
	}

	return string(body), nil
}

func (s Service) IdentityTemplates(envID, identityID string) (string, error) {
	baseURL := fmt.Sprintf("%s/api/1.0/identity-templates/%s/%s", s.cfg.PlainID.BaseURL, envID, identityID)
	req, err := http.NewRequest("GET", baseURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to download apps for %s: %s %s", envID, resp.Status, body)
	}

	return string(body), nil
}

type AppCaller[T any] struct {
	client *http.Client
}

func NewAppCaller[T any](client *http.Client) *AppCaller[T] {
	return &AppCaller[T]{
		client: client,
	}
}
func (a AppCaller[T]) Call(baseURL string) (*T, error) {
	req, err := http.NewRequest("GET", baseURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to call %s: %s %s", baseURL, resp.Status, body)
	}

	log.Debug().Msgf("Response from %s: %s", baseURL, string(body))

	var appResponse T
	err = json.Unmarshal(body, &appResponse)
	if err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &appResponse, nil
}

func (s Service) PAAGroups(envID string) ([]PAAGroup, error) {
	type paaGroupsResp struct {
		Data []PAAGroup `json:"data"`
	}

	baseURL := fmt.Sprintf("%s/api/1.0/paa-groups/%s?limit=10000&detailed=true", s.cfg.PlainID.BaseURL, envID)

	paaGroups, err := NewAppCaller[paaGroupsResp](s.client).Call(baseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch PAA groups for %s: %w", envID, err)
	}

	// Fetch sources and views for each group
	for i, paaGroup := range paaGroups.Data {
		// Fetch sources for this group
		type paaGroupsSourcesResp struct {
			Data []PAAGroupSource `json:"data"`
		}

		baseURL := fmt.Sprintf("%s/api/1.0/paa-groups/%s/%s/sources?limit=1000&detailed=true", s.cfg.PlainID.BaseURL, envID, paaGroup.ID)

		paaGroupSources, err := NewAppCaller[paaGroupsSourcesResp](s.client).Call(baseURL)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch PAA group sources for %s: %w", paaGroup.ID, err)
		}

		// Assign sources directly to the group
		paaGroups.Data[i].Sources = paaGroupSources.Data

		// Fetch views for this group
		type paaGroupsViewsResp struct {
			Data []PAAGroupViews `json:"data"`
		}

		baseURL = fmt.Sprintf("%s/api/1.0/paa-groups/%s/%s/views", s.cfg.PlainID.BaseURL, envID, paaGroup.ID)

		paaGroupViews, err := NewAppCaller[paaGroupsViewsResp](s.client).Call(baseURL)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch PAA group views for %s: %w", paaGroup.ID, err)
		}

		// Assign views directly to the group
		paaGroups.Data[i].Views = paaGroupViews.Data
	}

	return paaGroups.Data, nil
}

type PAAGroupTranslator struct {
	Name       string         `json:"name"`
	Type       string         `json:"type"`
	Properties map[string]any `json:"properties"`
}

type PAAGroupModel struct {
	Type        string           `json:"type"`
	ModelID     string           `json:"modelId"`
	PAAGroupID  string           `json:"paaGroupId"`
	IsVisible   bool             `json:"visible"`
	Name        string           `json:"name"`
	Description string           `json:"description"`
	SourceIDs   []string         `json:"sourceIds"`
	Properties  map[string]any   `json:"properties"`
	Metadata    []map[string]any `json:"metadata"`
}

type PAAGroupSource struct {
	ID             string             `json:"sourceId"`
	PAAGroupID     string             `json:"paaGroupId"`
	Adapter        string             `json:"adapter"`
	Name           string             `json:"name"`
	Status         string             `json:"status"`
	Description    string             `json:"description"`
	Properties     map[string]any     `json:"properties"`
	Translator     PAAGroupTranslator `json:"translator"`
	Models         []PAAGroupModel    `json:"models"`
	PaasProperties []map[string]any   `json:"paasProperties"`
}

type PAAGroupViews struct {
	Type  string `json:"type"`
	PAAID string `json:"paaId"`
	Text  string `json:"text"`
}

type PAAGroup struct {
	ID                  string           `json:"id"`
	PAAGroupType        string           `json:"paaGroupType"`
	PAAsCount           int              `json:"paasCount"`
	HasInactiveSyncPaas bool             `json:"hasInactiveSyncPaas"`
	Views               []PAAGroupViews  `json:"views"`
	Sources             []PAAGroupSource `json:"sources"`
}

func (p PAAGroup) ToJSON() (string, error) {
	b, err := json.Marshal(p)
	if err != nil {
		return "", fmt.Errorf("failed to marshal PAAGroup to JSON: %w", err)
	}
	return string(b), nil
}
