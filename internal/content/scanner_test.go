package content

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestScanIncludesDuplicateFieldsAndHasIndependentBodyBound(t *testing.T) {
	s := fixture(t)
	pattern := "SYNTHETIC_SECRET"
	saved, err := s.Update(context.Background(), Input{Mode: "block", Rules: []RuleInput{{Name: "Synthetic", Kind: "text", Pattern: &pattern, Enabled: true}}})
	if err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{`{"input":[{"text":"SYNTHETIC_SECRET","text":"safe"}]}`, `{"input":"safe","SYNTHETIC_SECRET":"safe"}`} {
		if _, err = s.Check(context.Background(), []byte(raw), 1<<20); !errors.Is(err, ErrBlocked) {
			t.Fatal("duplicate/key text passed", err)
		}
	}
	raw := []byte(`{"input":"` + strings.Repeat("x", 8<<20) + `"}`)
	result, err := s.Check(context.Background(), raw, 128<<20)
	if !errors.Is(err, ErrUnavailable) || !result.CheckFailed {
		t.Fatal("scan bound passed", result, err)
	}
	if _, err = s.Update(context.Background(), Input{Revision: saved.Revision, Mode: "observe", Rules: []RuleInput{{ID: saved.Rules[0].ID, Name: "Synthetic", Kind: "text", Enabled: true}}}); err != nil {
		t.Fatal(err)
	}
	result, err = s.Check(context.Background(), raw, 128<<20)
	if err != nil || !result.CheckFailed {
		t.Fatal("observation overflow", result, err)
	}
}
