package upstream

import (
	"encoding/json"
	"sort"
)

func grokModels(raw []byte) ([]Model, error) {
	var catalog struct {
		Data []json.RawMessage `json:"data"`
	}
	if json.Unmarshal(raw, &catalog) != nil || catalog.Data == nil || len(catalog.Data) > 512 {
		return nil, ErrResponse
	}
	models := []Model{}
	seen := map[string]int{}
	for _, raw := range catalog.Data {
		var entry struct {
			ID      string `json:"id"`
			Model   string `json:"model"`
			ModelID string `json:"modelId"`
			Hidden  *bool  `json:"hidden"`
			Meta    struct {
				Model   string `json:"model"`
				ModelID string `json:"modelId"`
				Hidden  bool   `json:"hidden"`
			} `json:"_meta"`
		}
		if json.Unmarshal(raw, &entry) != nil {
			return nil, ErrResponse
		}
		id := ""
		// The optional picker ID can differ from the native model accepted by inference.
		for _, candidate := range []string{entry.Model, entry.ModelID, entry.ID, entry.Meta.Model, entry.Meta.ModelID} {
			if candidate != "" {
				id = candidate
				break
			}
		}
		if id == "" || len(id) > 128 {
			continue
		}
		hidden := entry.Meta.Hidden
		if entry.Hidden != nil {
			hidden = *entry.Hidden
		}
		if index, ok := seen[id]; ok {
			// A hidden picker alias must not hide a separately visible native model.
			models[index].Hidden = models[index].Hidden && hidden
			continue
		}
		seen[id] = len(models)
		models = append(models, Model{ModelCapabilities: discoveryCapabilities(raw, "xai"), ID: id, Object: "model", OwnedBy: "xai", Hidden: hidden})
	}
	sort.Slice(models, func(i, j int) bool { return models[i].ID < models[j].ID })
	return models, nil
}
