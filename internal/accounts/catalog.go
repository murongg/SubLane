package accounts

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"regexp"
	"slices"
	"sort"

	"github.com/murongg/SubLane/internal/storage/db"
)

var ErrCatalogChanged = errors.New("account_catalog_changed")

var catalogModelID = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._:/()+-]{0,127}$`)

// UpdatedAt zero means unknown; an observed empty list is a known catalog.
type Catalog struct {
	Models       []string                     `json:"models"`
	Capabilities map[string]ModelCapabilities `json:"capabilities,omitempty"`
	HiddenModels []string                     `json:"hidden_models,omitempty"`
	UpdatedAt    int64                        `json:"updated_at"`
	Source       string                       `json:"source,omitempty"`
	Revision     int64                        `json:"-"`
}

// Visibility controls discovery lists, never the persisted set of callable models.
func (c Catalog) VisibleModels() []string {
	return slices.DeleteFunc(slices.Clone(c.Models), func(model string) bool { return slices.Contains(c.HiddenModels, model) })
}

func (s *Service) Catalog(ctx context.Context, id string) (Catalog, error) {
	if _, err := s.get(ctx, id); err != nil {
		return Catalog{}, err
	}
	row, err := s.queries.GetAccountCatalog(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return Catalog{}, ErrNotFound
	}
	if err != nil {
		return Catalog{}, err
	}
	return DecodeCatalog(row.ModelsSnapshot, row.ModelsRevision)
}

func DecodeCatalog(raw []byte, revision int64) (Catalog, error) {
	value := Catalog{Models: []string{}, Revision: revision}
	if len(raw) == 0 {
		return value, nil
	}
	// The snapshot includes bounded optional metadata for up to 512 models.
	if len(raw) > 512<<10 || json.Unmarshal(raw, &value) != nil || value.UpdatedAt <= 0 || value.Models == nil || len(value.Source) > 64 {
		return Catalog{}, ErrInput
	}
	models, err := normalizedCatalog(value.Models)
	if err != nil {
		return Catalog{}, err
	}
	value.Models = models
	value.HiddenModels, err = normalizedHiddenModels(models, value.HiddenModels)
	if err != nil {
		return Catalog{}, err
	}
	value.Capabilities, err = normalizedCapabilities(models, value.Capabilities)
	return value, err
}

func normalizedHiddenModels(models, hidden []string) ([]string, error) {
	hidden, err := normalizedCatalog(hidden)
	if err != nil {
		return nil, err
	}
	for _, model := range hidden {
		if !slices.Contains(models, model) {
			return nil, ErrInput
		}
	}
	return hidden, nil
}

func normalizedCatalog(models []string) ([]string, error) {
	if len(models) > 512 {
		return nil, ErrInput
	}
	seen := make(map[string]bool, len(models))
	result := make([]string, 0, len(models))
	for _, id := range models {
		if !catalogModelID.MatchString(id) {
			return nil, ErrInput
		}
		if !seen[id] {
			seen[id] = true
			result = append(result, id)
		}
	}
	sort.Strings(result)
	return result, nil
}

func (s *Service) SaveCatalog(ctx context.Context, id string, revision int64, models []string, observedAt int64, source string, hiddenModels ...string) error {
	return s.SaveCatalogSnapshot(ctx, id, revision, Catalog{Models: models, HiddenModels: hiddenModels, UpdatedAt: observedAt, Source: source})
}

func (s *Service) SaveCatalogSnapshot(ctx context.Context, id string, revision int64, snapshot Catalog) error {
	if _, err := s.get(ctx, id); err != nil {
		return err
	}
	if snapshot.UpdatedAt <= 0 || len(snapshot.Source) > 64 {
		return ErrInput
	}
	models, err := normalizedCatalog(snapshot.Models)
	if err != nil {
		return err
	}
	snapshot.Models = models
	snapshot.HiddenModels, err = normalizedHiddenModels(models, snapshot.HiddenModels)
	if err != nil {
		return err
	}
	snapshot.Capabilities, err = normalizedCapabilities(models, snapshot.Capabilities)
	if err != nil {
		return err
	}
	raw, err := json.Marshal(snapshot)
	if err != nil {
		return err
	}
	if len(raw) > 512<<10 {
		return ErrInput
	}
	// The conditional write prevents a late refresh from reviving pre-authorization or disabled data.
	changed, err := s.queries.SaveAccountCatalog(ctx, db.SaveAccountCatalogParams{ID: id, Revision: revision, Snapshot: raw, ObservedAt: snapshot.UpdatedAt})
	if err != nil {
		return err
	}
	if changed != 1 {
		return ErrCatalogChanged
	}
	return nil
}
