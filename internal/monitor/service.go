package monitor

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"withdraw-bot/internal/core"
	"withdraw-bot/internal/storage"
)

const moduleIDEvidenceKey = "module_id"

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
	if clock == nil {
		clock = core.SystemClock{}
	}
	return &Service{Modules: modules, Storage: repos, Clock: clock, Latest: map[core.MonitorModuleID]core.MonitorResult{}}
}

func (service *Service) RunOnce(ctx context.Context) ([]core.MonitorResult, error) {
	results := make([]core.MonitorResult, 0, len(service.Modules))
	var resultErr error
	for _, module := range service.Modules {
		if module == nil {
			return results, errors.Join(resultErr, errors.New("monitor module is nil"))
		}
		moduleID := module.ID()
		if moduleID == "" {
			return results, errors.Join(resultErr, errors.New("monitor module id is empty"))
		}
		result, err := module.Monitor(ctx)
		if err != nil {
			resultErr = errors.Join(resultErr, fmt.Errorf("run monitor module %s: %w", moduleID, err))
			result = core.MonitorResult{
				ModuleID:   moduleID,
				Status:     core.MonitorStatusUnknown,
				ObservedAt: service.Clock.Now(),
				Findings: []core.Finding{{
					Key:      core.FindingStaleData,
					Severity: core.SeverityWarn,
					Message:  fmt.Sprintf("module %s failed: %v", moduleID, err),
					Evidence: map[string]string{moduleIDEvidenceKey: string(moduleID)},
				}},
			}
		}
		result.ModuleID = moduleID
		resultClone := result.Clone()
		service.mu.Lock()
		service.Latest[resultClone.ModuleID] = resultClone
		service.mu.Unlock()
		if err := service.Storage.InsertMonitorResult(ctx, resultClone.Clone(), service.Clock.Now()); err != nil {
			resultErr = errors.Join(resultErr, fmt.Errorf("store monitor result for %s: %w", moduleID, err))
		}
		results = append(results, resultClone.Clone())
	}
	return results, resultErr
}

func (service *Service) Snapshot() map[core.MonitorModuleID]core.MonitorResult {
	service.mu.RLock()
	defer service.mu.RUnlock()
	result := make(map[core.MonitorModuleID]core.MonitorResult, len(service.Latest))
	for key, value := range service.Latest {
		result[key] = value.Clone()
	}
	return result
}

func (service *Service) RunLoop(ctx context.Context, interval time.Duration) error {
	if interval <= 0 {
		return fmt.Errorf("monitor interval must be positive")
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	if _, err := service.RunOnce(ctx); err != nil {
		return err
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if _, err := service.RunOnce(ctx); err != nil {
				return err
			}
		}
	}
}
