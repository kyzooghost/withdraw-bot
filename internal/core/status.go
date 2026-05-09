package core

type Severity string
type MonitorStatus string

const (
	SeverityWarn   Severity = "warn"
	SeverityUrgent Severity = "urgent"
)

const (
	MonitorStatusOK      MonitorStatus = "OK"
	MonitorStatusWarn    MonitorStatus = "WARN"
	MonitorStatusUrgent  MonitorStatus = "URGENT"
	MonitorStatusUnknown MonitorStatus = "UNKNOWN"
)

func WorstKnownStatus(statuses []MonitorStatus) MonitorStatus {
	worst := MonitorStatusOK
	seenKnown := false
	for _, status := range statuses {
		switch status {
		case MonitorStatusUrgent:
			return MonitorStatusUrgent
		case MonitorStatusWarn:
			seenKnown = true
			worst = MonitorStatusWarn
		case MonitorStatusOK:
			seenKnown = true
		}
	}
	if seenKnown {
		return worst
	}
	return MonitorStatusUnknown
}
