package core

import "context"

// MonitorModule is the interface that all protocol monitor modules implement.
type MonitorModule interface {
	ID() MonitorModuleID
	ValidateConfig(ctx context.Context) error
	Bootstrap(ctx context.Context) (map[string]any, error)
	Monitor(ctx context.Context) (MonitorResult, error)
}

// ExitSimulator provides position and exit simulation data for monitoring modules.
type ExitSimulator interface {
	Position(ctx context.Context) (PositionSnapshot, error)
	SimulateFullExit(ctx context.Context, req FullExitRequest) (FullExitSimulation, error)
}
