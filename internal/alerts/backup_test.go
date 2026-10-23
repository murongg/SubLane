package alerts

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestBackupFailureUsesBoundedInstanceNotification(t *testing.T) {
	calls := 0
	s, _ := fixture(t, senderFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if _, ok := r.Context().Deadline(); !ok {
			t.Fatal("notification lacks deadline")
		}
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(raw), "synthetic-destination-secret") {
			t.Fatal("destination leaked")
		}
		var event Event
		if err := json.Unmarshal(raw, &event); err != nil || event.Kind != "backup_failure" || event.Subject != "instance" || event.WorkspaceID != 0 || event.Status != "firing" || len(event.ID) != 32 {
			t.Fatal(event, err)
		}
		return &http.Response{StatusCode: 204, Body: io.NopCloser(strings.NewReader(""))}, nil
	}))
	if err := s.BackupFailure(context.Background(), "https://hooks.example.test/backup?token=synthetic-destination-secret"); err != nil {
		t.Fatal(err)
	}
	if err := s.BackupFailure(context.Background(), "https://127.0.0.1/backup"); err == nil {
		t.Fatal("private webhook accepted")
	}
	if calls != 1 {
		t.Fatal("invalid destination reached sender", calls)
	}
}
