// Package hardware exposes a per-OS view of the physical state of the
// machine SolderDB is running on. The engine consults this before kicking
// off heavy background work (compaction, snapshots) so the database can
// step aside when the user is on battery or the CPU is thermal-throttled.
//
// Per-OS implementations live in monitor_*.go files behind build tags.
// All fields are optional — a value of nil/-1 means "couldn't read".
package hardware

import "time"

type Status struct {
	// Battery
	OnBattery   bool    `json:"onBattery"`
	BatteryPct  int     `json:"batteryPct"`  // 0..100, -1 if unknown
	BatteryKnown bool   `json:"batteryKnown"`

	// Thermal
	CPUTempC    float64 `json:"cpuTempC"`     // 0 if unknown
	CPUTempKnown bool   `json:"cpuTempKnown"`

	// Identifying which OS reported this
	Platform string `json:"platform"`

	// When this snapshot was taken.
	Timestamp string `json:"timestamp"`
}

// Thresholds expresses the rules a Monitor uses to decide whether the
// engine should throttle background work. Zero values mean "ignore".
type Thresholds struct {
	// MinBatteryPct — if on battery and pct < this, throttle. 0 disables.
	MinBatteryPct int `json:"minBatteryPct"`

	// MaxCPUTempC — if temp > this, throttle. 0 disables.
	MaxCPUTempC float64 `json:"maxCpuTempC"`

	// PauseOnBattery — if true, any time the machine is unplugged, throttle.
	PauseOnBattery bool `json:"pauseOnBattery"`
}

func DefaultThresholds() Thresholds {
	return Thresholds{
		MinBatteryPct:  20,
		MaxCPUTempC:    85,
		PauseOnBattery: false,
	}
}

// ShouldThrottle returns true if the current status meets any throttle
// condition. The reason string is suitable for surfacing in the UI.
func (s Status) ShouldThrottle(t Thresholds) (bool, string) {
	if t.PauseOnBattery && s.OnBattery {
		return true, "running on battery"
	}
	if t.MinBatteryPct > 0 && s.OnBattery && s.BatteryKnown && s.BatteryPct < t.MinBatteryPct {
		return true, "battery low (" + itoa(s.BatteryPct) + "%)"
	}
	if t.MaxCPUTempC > 0 && s.CPUTempKnown && s.CPUTempC > t.MaxCPUTempC {
		return true, "CPU hot (" + ftoa(s.CPUTempC) + "°C)"
	}
	return false, ""
}

// Monitor reads the current hardware status. Cheap to call (well under 1ms);
// safe for concurrent callers.
type Monitor interface {
	Read() Status
}

// New returns the platform-appropriate monitor. Falls back to a stub that
// always reports "unknown" if no implementation matches.
func New() Monitor {
	return platformMonitor()
}

// ---------------- tiny stdlib-only helpers ----------------

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := false
	if n < 0 {
		neg = true
		n = -n
	}
	var buf [12]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

func ftoa(f float64) string {
	// One-decimal-place fixed format, no allocations beyond the return.
	whole := int(f)
	frac := int((f - float64(whole)) * 10)
	if frac < 0 {
		frac = -frac
	}
	return itoa(whole) + "." + itoa(frac)
}

func nowIso() string { return time.Now().UTC().Format(time.RFC3339) }
