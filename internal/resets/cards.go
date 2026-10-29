package resets

import (
	"slices"
	"sort"
	"time"
)

type Card struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Status    string `json:"status"`
	Supported bool   `json:"supported"`
	GrantedAt int64  `json:"granted_at"`
	ExpiresAt *int64 `json:"expires_at"`
}

type CardList struct {
	Cards          []Card `json:"cards"`
	AvailableCount *int64 `json:"available_count"`
	ObservedAt     int64  `json:"observed_at"`
	NextID         string `json:"next_card_id"`
}

func (c Card) available(now int64) bool {
	return c.Supported && c.Status == "available" && (c.ExpiresAt == nil || *c.ExpiresAt > now)
}

// PrepareCards is shared by the displayed priority and the actual redemption decision.
func PrepareCards(value CardList, now time.Time) CardList {
	value.Cards = slices.Clone(value.Cards)
	if value.Cards == nil {
		value.Cards = []Card{}
	}
	for i, card := range value.Cards {
		if card.Status == "available" && card.ExpiresAt != nil && *card.ExpiresAt <= now.Unix() {
			value.Cards[i].Status = "expired"
		}
	}
	sort.Slice(value.Cards, func(i, j int) bool {
		a, b := value.Cards[i], value.Cards[j]
		if a.available(now.Unix()) != b.available(now.Unix()) {
			return a.available(now.Unix())
		}
		if (a.ExpiresAt == nil) != (b.ExpiresAt == nil) {
			return a.ExpiresAt != nil
		}
		if a.ExpiresAt != nil && *a.ExpiresAt != *b.ExpiresAt {
			return *a.ExpiresAt < *b.ExpiresAt
		}
		if a.GrantedAt != b.GrantedAt {
			return a.GrantedAt < b.GrantedAt
		}
		return a.ID < b.ID
	})
	value.NextID = ""
	if len(value.Cards) > 0 && value.Cards[0].available(now.Unix()) {
		value.NextID = value.Cards[0].ID
	}
	return value
}
