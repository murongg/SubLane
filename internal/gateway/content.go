package gateway

import (
	"context"
	"encoding/json"

	"github.com/murongg/SubLane/internal/content"
	"github.com/murongg/SubLane/internal/upstream"
)

func (s *Service) checkContent(ctx context.Context, entry *observation, raw []byte) error {
	if s.content == nil {
		return nil
	}
	result, err := s.content.Check(ctx, raw, s.MaxRequestBody(), entry.record.Model)
	applyContent(entry, result)
	return err
}
func applyContent(entry *observation, result content.Result) {
	entry.suppressModel = result.MetadataMatched || result.CheckFailed
	entry.record.ContentMode = result.Mode
	entry.record.ContentRevision = result.Revision
	ids, _ := json.Marshal(result.RuleIDs)
	entry.record.ContentRuleIds = string(ids)
	if result.CheckFailed {
		entry.record.ContentCheckFailed = 1
	}
}

func (s *Service) CheckPrewarm(ctx context.Context, userID, groupID int64, raw []byte) error {
	if s.content == nil {
		return nil
	}
	var input struct {
		Model string `json:"model"`
	}
	if json.Unmarshal(raw, &input) != nil {
		return upstream.ErrInput
	}
	result, err := s.content.Check(ctx, raw, s.MaxRequestBody(), input.Model)
	if err == nil {
		return nil
	}
	entry, beginErr := s.begin(ctx, userID, groupID, Responses)
	if beginErr != nil {
		return beginErr
	}
	entry.record.Model = input.Model
	applyContent(entry, result)
	entry.fail(err)
	return err
}
