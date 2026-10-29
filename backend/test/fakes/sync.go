package fakes

import (
	"context"
	"sync"

	"github.com/google/uuid"

	"gp/internal/connector"
)

// Schedules is a fake of the per-connector recurring schedule: it remembers
// which connectors have one and with what interval.
type Schedules struct {
	mu   sync.Mutex
	live map[uuid.UUID]int
}

func NewSchedules() *Schedules { return &Schedules{live: map[uuid.UUID]int{}} }

func (s *Schedules) Upsert(_ context.Context, id uuid.UUID, intervalMin int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.live[id] = intervalMin
	return nil
}

func (s *Schedules) Delete(_ context.Context, id uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.live, id)
	return nil
}

// Interval is the interval of the connector's schedule, and whether it has one.
func (s *Schedules) Interval(id uuid.UUID) (int, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	n, ok := s.live[id]
	return n, ok
}

// Invoker is a fake of the async invoke of the sync function: it only keeps the
// jobs, and a test runs them with connector.Service.Run when it wants to.
type Invoker struct {
	mu   sync.Mutex
	jobs []connector.Job
}

func (i *Invoker) Invoke(_ context.Context, job connector.Job) error {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.jobs = append(i.jobs, job)
	return nil
}

// Jobs are the jobs started so far, in order.
func (i *Invoker) Jobs() []connector.Job {
	i.mu.Lock()
	defer i.mu.Unlock()
	return append([]connector.Job(nil), i.jobs...)
}
