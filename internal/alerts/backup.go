package alerts

import (
	"context"
	"time"
)

// BackupFailure is an operator-configured instance event, independent of workspace permissions and incidents.
func (s *Service) BackupFailure(ctx context.Context, endpoint string) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return deliver(ctx, s.sender, endpoint, Event{ID: newEventID(), Kind: "backup_failure", Subject: "instance", Status: "firing", ObservedAt: s.now().Unix()})
}
