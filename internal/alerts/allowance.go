package alerts

import "context"

type Allowance struct {
	Subject string
	Cycle   string
}

func (s *Service) SetAllowanceObserver(observer func(context.Context, int64) ([]Allowance, error)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.allowanceObserver = observer
}
