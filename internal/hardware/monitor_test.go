package hardware

import "testing"

func TestNewReturnsSomething(t *testing.T) {
	m := New()
	if m == nil {
		t.Fatal("New() returned nil")
	}
	s := m.Read()
	if s.Platform == "" {
		t.Fatal("Read() returned empty Platform")
	}
	if s.Timestamp == "" {
		t.Fatal("Read() returned empty Timestamp")
	}
}

func TestShouldThrottleRules(t *testing.T) {
	cases := []struct {
		name     string
		status   Status
		thresh   Thresholds
		wantStop bool
	}{
		{
			name:     "battery low triggers throttle",
			status:   Status{OnBattery: true, BatteryKnown: true, BatteryPct: 15},
			thresh:   Thresholds{MinBatteryPct: 20},
			wantStop: true,
		},
		{
			name:     "plugged in with low battery doesn't throttle",
			status:   Status{OnBattery: false, BatteryKnown: true, BatteryPct: 15},
			thresh:   Thresholds{MinBatteryPct: 20},
			wantStop: false,
		},
		{
			name:     "hot CPU triggers throttle",
			status:   Status{CPUTempKnown: true, CPUTempC: 90},
			thresh:   Thresholds{MaxCPUTempC: 85},
			wantStop: true,
		},
		{
			name:     "unknown CPU temp does not trigger temp rule",
			status:   Status{CPUTempKnown: false, CPUTempC: 999},
			thresh:   Thresholds{MaxCPUTempC: 85},
			wantStop: false,
		},
		{
			name:     "pause-on-battery flag triggers regardless of pct",
			status:   Status{OnBattery: true, BatteryKnown: true, BatteryPct: 100},
			thresh:   Thresholds{PauseOnBattery: true},
			wantStop: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, _ := tc.status.ShouldThrottle(tc.thresh)
			if got != tc.wantStop {
				t.Fatalf("ShouldThrottle = %v, want %v", got, tc.wantStop)
			}
		})
	}
}
