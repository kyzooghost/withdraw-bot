package app

type Mode string

const (
	ModeMonitor     Mode = "monitor"
	ModeBootstrap   Mode = "bootstrap"
	ModeConfigCheck Mode = "config-check"
)

func ParseMode(value string) (Mode, bool) {
	switch Mode(value) {
	case ModeMonitor, ModeBootstrap, ModeConfigCheck:
		return Mode(value), true
	default:
		return "", false
	}
}
