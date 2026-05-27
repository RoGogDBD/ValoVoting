package poll

import "sync"

type Service struct {
	mu    sync.RWMutex
	state State
}

func NewService() *Service {
	return &Service{state: IdleState()}
}

func (s *Service) GetState() State {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.state
}

func (s *Service) SetState(st State) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state = st
}
