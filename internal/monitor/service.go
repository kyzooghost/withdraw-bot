package monitor

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"time"

	"withdraw-bot/internal/core"
	"withdraw-bot/internal/storage"
)

const moduleIDEvidenceKey = "module_id"

var (
	errInvalidMonitorModule = errors.New("invalid monitor module")
	errMonitorModuleFailure = errors.New("monitor module failure")
	errMonitorResultHandler = errors.New("monitor result handler failure")
	errMonitorStorage       = errors.New("monitor storage failure")
)

type Module interface {
	ID() core.MonitorModuleID
	ValidateConfig(ctx context.Context) error
	Bootstrap(ctx context.Context) (map[string]any, error)
	Monitor(ctx context.Context) (core.MonitorResult, error)
}

type ResultHandler interface {
	HandleMonitorResults(ctx context.Context, results []core.MonitorResult) error
}

type Service struct {
	Modules       []Module
	Storage       storage.Repositories
	Clock         core.Clock
	Latest        map[core.MonitorModuleID]core.MonitorResult
	ResultHandler ResultHandler
	mu            sync.RWMutex
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
		if isNilModule(module) {
			return results, errors.Join(resultErr, errInvalidMonitorModule, errors.New("monitor module is nil"))
		}
		moduleID := module.ID()
		if moduleID == "" {
			return results, errors.Join(resultErr, errInvalidMonitorModule, errors.New("monitor module id is empty"))
		}
		if err := module.ValidateConfig(ctx); err != nil {
			return results, errors.Join(resultErr, errInvalidMonitorModule, fmt.Errorf("validate monitor module %s: %w", moduleID, err))
		}
		result, err := module.Monitor(ctx)
		if err != nil {
			resultErr = errors.Join(resultErr, errMonitorModuleFailure, fmt.Errorf("run monitor module %s: %w", moduleID, err))
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
			resultErr = errors.Join(resultErr, errMonitorStorage, fmt.Errorf("store monitor result for %s: %w", moduleID, err))
		}
		results = append(results, resultClone.Clone())
	}
	if service.ResultHandler != nil && shouldHandleMonitorResults(resultErr) {
		if err := service.ResultHandler.HandleMonitorResults(ctx, cloneMonitorResults(results)); err != nil {
			resultErr = errors.Join(resultErr, errMonitorResultHandler, err)
		}
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
	if _, err := service.RunOnce(ctx); err != nil && !isRecoverableMonitorError(err) {
		return err
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if _, err := service.RunOnce(ctx); err != nil && !isRecoverableMonitorError(err) {
				return err
			}
		}
	}
}

func isRecoverableMonitorError(err error) bool {
	if errors.Is(err, errInvalidMonitorModule) || errors.Is(err, errMonitorStorage) {
		return false
	}
	return errors.Is(err, errMonitorModuleFailure) || errors.Is(err, errMonitorResultHandler)
}

func shouldHandleMonitorResults(err error) bool {
	return !errors.Is(err, errInvalidMonitorModule) && !errors.Is(err, errMonitorStorage)
}

func cloneMonitorResults(results []core.MonitorResult) []core.MonitorResult {
	if results == nil {
		return nil
	}
	clone := make([]core.MonitorResult, len(results))
	for index, result := range results {
		clone[index] = result.Clone()
	}
	return clone
}

func isNilModule(module Module) bool {
	if module == nil {
		return true
	}
	value := reflect.ValueOf(module)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}
