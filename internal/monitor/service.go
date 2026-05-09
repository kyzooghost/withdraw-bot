package monitor

import (
	"context"
	"fmt"
	"sync"
	"time"

	"withdraw-bot/internal/core"
	"withdraw-bot/internal/storage"
)

type Module interface {
	ID() core.MonitorModuleID
	ValidateConfig(ctx context.Context) error
	Bootstrap(ctx context.Context) (map[string]any, error)
	Monitor(ctx context.Context) (core.MonitorResult, error)
}

type Service struct {
	Modules []Module
	Storage storage.Repositories
	Clock   core.Clock
	Latest  map[core.MonitorModuleID]core.MonitorResult
	mu      sync.RWMutex
}

func NewService(modules []Module, repos storage.Repositories, clock core.Clock) *Service {
	return &Service{Modules: modules, Storage: repos, Clock: clock, Latest: map[core.MonitorModuleID]core.MonitorResult{}}
}

func (service *Service) RunOnce(ctx context.Context) []core.MonitorResult {
	results := make([]core.MonitorResult, 0, len(service.Modules))
	for _, module := range service.Modules {
		result, err := module.Monitor(ctx)
		if err != nil {
			result = core.MonitorResult{
				ModuleID:   module.ID(),
				Status:     core.MonitorStatusUnknown,
				ObservedAt: service.Clock.Now(),
				Findings: []core.Finding{{
					Key:      core.FindingStaleData,
					Severity: core.SeverityWarn,
					Message:  fmt.Sprintf("module %s failed: %v", module.ID(), err),
					Evidence: map[string]string{"module_id": string(module.ID())},
				}},
			}
		}
		service.mu.Lock()
		service.Latest[result.ModuleID] = result
		service.mu.Unlock()
		_ = service.Storage.InsertMonitorResult(ctx, result, service.Clock.Now())
		results = append(results, result)
	}
	return results
}

func (service *Service) Snapshot() map[core.MonitorModuleID]core.MonitorResult {
	service.mu.RLock()
	defer service.mu.RUnlock()
	result := make(map[core.MonitorModuleID]core.MonitorResult, len(service.Latest))
	for key, value := range service.Latest {
		result[key] = value
	}
	return result
}

func (service *Service) RunLoop(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	service.RunOnce(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			service.RunOnce(ctx)
		}
	}
}
