package upstream

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/murongg/SubLane/internal/accounts"
)

func channelModelsURL(credential accounts.Credential) string {
	base := credential.BaseURL
	config := credential.APIConfig()
	if (config.Platform == "qwen" || config.Platform == "qwen-cn") && strings.HasSuffix(base, "/compatible-mode/v1") {
		return strings.TrimSuffix(base, "/compatible-mode/v1") + "/api/v1/models"
	}
	if config.Platform == "opencode" || config.Platform == "opencode-go" {
		for _, suffix := range []string{"/openai/v1", "/anthropic/v1", "/google/v1beta"} {
			if strings.HasSuffix(base, suffix) {
				return strings.TrimSuffix(base, suffix) + "/v1/models"
			}
		}
	}
	switch config.Protocol {
	case "anthropic":
		if !strings.HasSuffix(base, "/v1") {
			base += "/v1"
		}
	case "gemini":
		if !strings.HasSuffix(base, "/v1beta") {
			base += "/v1beta"
		}
	}
	return base + "/models"
}

func (c *Client) channelModels(ctx context.Context, credential accounts.Credential) ([]Model, error) {
	executor, err := c.executor("openai")
	if err != nil {
		return nil, err
	}
	endpoint, _ := url.Parse(channelModelsURL(credential))
	protocol := credential.APIConfig().Protocol
	dashscope := credential.APIConfig().Platform == "qwen" || credential.APIConfig().Platform == "qwen-cn"
	query := endpoint.Query()
	if protocol == "anthropic" {
		query.Set("limit", "100")
	}
	if protocol == "gemini" {
		query.Set("pageSize", "100")
	}
	if dashscope {
		query.Set("page_size", "100")
		query.Set("capabilities", "TG")
	}
	models := []Model{}
	seen, pages := map[string]bool{}, map[string]bool{}
	for pageNumber := range 8 {
		if dashscope {
			query.Set("page_no", strconv.Itoa(pageNumber+1))
		}
		endpoint.RawQuery = query.Encode()
		if pages[endpoint.String()] {
			return nil, ErrResponse
		}
		pages[endpoint.String()] = true
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
		if err != nil {
			return nil, ErrInput
		}
		request.Header.Set("Accept", "application/json")
		response, err := executor.HttpRequest(c.sdkContext(ctx, credential), channelAuth(credential), request)
		if err != nil {
			return nil, sdkError(err)
		}
		if response.StatusCode != http.StatusOK {
			response.Body.Close()
			return nil, &UpstreamError{Status: response.StatusCode, RetryAfter: response.Header.Get("Retry-After")}
		}
		raw, err := readBounded(response.Body, 2<<20)
		response.Body.Close()
		if err != nil {
			return nil, err
		}
		var page struct {
			Success *bool `json:"success"`
			Output  *struct {
				Total  int `json:"total"`
				Models []struct {
					ID string `json:"model"`
				} `json:"models"`
			} `json:"output"`
			Data []struct {
				ID string `json:"id"`
			} `json:"data"`
			Models []struct {
				Name    string   `json:"name"`
				Methods []string `json:"supportedGenerationMethods"`
			} `json:"models"`
			HasMore  bool   `json:"has_more"`
			LastID   string `json:"last_id"`
			NextPage string `json:"nextPageToken"`
		}
		if json.Unmarshal(raw, &page) != nil {
			return nil, ErrResponse
		}
		ids := []string{}
		if dashscope && page.Output != nil {
			if page.Success != nil && !*page.Success || page.Output.Total < 0 || page.Output.Total > 512 || len(page.Output.Models) > 512 {
				return nil, ErrResponse
			}
			for _, model := range page.Output.Models {
				ids = append(ids, model.ID)
			}
		} else if protocol == "gemini" {
			if page.Models == nil || len(page.Models) > 512 {
				return nil, ErrResponse
			}
			for _, model := range page.Models {
				if slices.Contains(model.Methods, "generateContent") {
					ids = append(ids, strings.TrimPrefix(model.Name, "models/"))
				}
			}
		} else {
			if page.Data == nil || len(page.Data) > 512 {
				return nil, ErrResponse
			}
			for _, model := range page.Data {
				ids = append(ids, model.ID)
			}
		}
		for _, id := range ids {
			if id != "" && len(id) <= 128 && !seen[id] {
				seen[id] = true
				models = append(models, Model{ID: id, Object: "model", OwnedBy: "openai"})
			}
		}
		if len(models) > 512 {
			return nil, ErrResponse
		}
		switch {
		case dashscope && page.Output != nil && page.Output.Total > len(models):
			if len(ids) == 0 {
				return nil, ErrResponse
			}
		case protocol == "gemini" && page.NextPage != "":
			if len(page.NextPage) > 4096 {
				return nil, ErrResponse
			}
			query.Set("pageToken", page.NextPage)
		case protocol == "anthropic" && page.HasMore:
			if page.LastID == "" || len(page.LastID) > 128 {
				return nil, ErrResponse
			}
			query.Set("after_id", page.LastID)
		default:
			sort.Slice(models, func(i, j int) bool { return models[i].ID < models[j].ID })
			return models, nil
		}
	}
	return nil, ErrResponse
}
