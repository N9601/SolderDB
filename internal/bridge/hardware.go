package bridge

import (
	"sync"

	"solderdb/internal/engine"
	"solderdb/internal/hardware"
)

// HardwareService bridges hardware-aware compaction policy into the UI:
// reading current status, viewing/editing thresholds.
type HardwareService struct {
	mu     sync.RWMutex
	mon    hardware.Monitor
	thresh hardware.Thresholds
	db     *engine.DB
}

func NewHardwareService(db *engine.DB) *HardwareService {
	s := &HardwareService{
		mon:    hardware.New(),
		thresh: hardware.DefaultThresholds(),
		db:     db,
	}
	db.SetCompactionGate(gateAdapter{svc: s})
	return s
}

// gateAdapter satisfies engine.CompactionGate by closing over the service so
// threshold updates take effect immediately.
type gateAdapter struct{ svc *HardwareService }

func (a gateAdapter) Allow() (bool, string) {
	a.svc.mu.RLock()
	defer a.svc.mu.RUnlock()
	if throttle, reason := a.svc.mon.Read().ShouldThrottle(a.svc.thresh); throttle {
		return false, reason
	}
	return true, ""
}

// --- DTOs for Wails ---

type HardwareStatus struct {
	OnBattery    bool    `json:"onBattery"`
	BatteryPct   int     `json:"batteryPct"`
	BatteryKnown bool    `json:"batteryKnown"`
	CPUTempC     float64 `json:"cpuTempC"`
	CPUTempKnown bool    `json:"cpuTempKnown"`
	Platform     string  `json:"platform"`
	Timestamp    string  `json:"timestamp"`
	Throttled    bool    `json:"throttled"`
	Reason       string  `json:"reason"`
}

type HardwareThresholds struct {
	MinBatteryPct  int     `json:"minBatteryPct"`
	MaxCPUTempC    float64 `json:"maxCpuTempC"`
	PauseOnBattery bool    `json:"pauseOnBattery"`
}

func (s *HardwareService) GetStatus() HardwareStatus {
	s.mu.RLock()
	cur := s.mon.Read()
	t := s.thresh
	s.mu.RUnlock()
	throttle, reason := cur.ShouldThrottle(t)
	return HardwareStatus{
		OnBattery:    cur.OnBattery,
		BatteryPct:   cur.BatteryPct,
		BatteryKnown: cur.BatteryKnown,
		CPUTempC:     cur.CPUTempC,
		CPUTempKnown: cur.CPUTempKnown,
		Platform:     cur.Platform,
		Timestamp:    cur.Timestamp,
		Throttled:    throttle,
		Reason:       reason,
	}
}

func (s *HardwareService) GetThresholds() HardwareThresholds {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return HardwareThresholds{
		MinBatteryPct:  s.thresh.MinBatteryPct,
		MaxCPUTempC:    s.thresh.MaxCPUTempC,
		PauseOnBattery: s.thresh.PauseOnBattery,
	}
}

func (s *HardwareService) SetThresholds(t HardwareThresholds) HardwareThresholds {
	s.mu.Lock()
	s.thresh = hardware.Thresholds{
		MinBatteryPct:  t.MinBatteryPct,
		MaxCPUTempC:    t.MaxCPUTempC,
		PauseOnBattery: t.PauseOnBattery,
	}
	out := s.thresh
	s.mu.Unlock()
	return HardwareThresholds{
		MinBatteryPct:  out.MinBatteryPct,
		MaxCPUTempC:    out.MaxCPUTempC,
		PauseOnBattery: out.PauseOnBattery,
	}
}
