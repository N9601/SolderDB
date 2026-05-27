//go:build !windows && !linux && !darwin

package hardware

func platformMonitor() Monitor { return &stubMonitor{} }

type stubMonitor struct{}

func (m *stubMonitor) Read() Status {
	return Status{Platform: "unknown", Timestamp: nowIso(), BatteryPct: -1}
}
