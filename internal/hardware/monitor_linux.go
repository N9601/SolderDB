//go:build linux

package hardware

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func platformMonitor() Monitor { return &linuxMonitor{} }

type linuxMonitor struct{}

func (m *linuxMonitor) Read() Status {
	st := Status{Platform: "linux", Timestamp: nowIso(), BatteryPct: -1}

	// Battery — look for the first BAT* power_supply entry.
	if entries, err := os.ReadDir("/sys/class/power_supply"); err == nil {
		for _, e := range entries {
			name := e.Name()
			if !strings.HasPrefix(name, "BAT") {
				continue
			}
			cap := readIntFile(filepath.Join("/sys/class/power_supply", name, "capacity"))
			status := strings.TrimSpace(readStringFile(filepath.Join("/sys/class/power_supply", name, "status")))
			if cap >= 0 {
				st.BatteryKnown = true
				st.BatteryPct = cap
			}
			// "Discharging" or "Not charging" → on battery.
			st.OnBattery = status == "Discharging" || status == "Not charging"
			break
		}
		// Cross-check via AC adapter if no battery info contradicted.
		for _, e := range entries {
			name := e.Name()
			if !(strings.HasPrefix(name, "AC") || strings.HasPrefix(name, "ADP")) {
				continue
			}
			online := readIntFile(filepath.Join("/sys/class/power_supply", name, "online"))
			if online == 0 {
				st.OnBattery = true
			} else if online == 1 {
				st.OnBattery = false
			}
			break
		}
	}

	// Thermal — take the hottest of all thermal zones we can read.
	if entries, err := os.ReadDir("/sys/class/thermal"); err == nil {
		var hottest int = -1
		for _, e := range entries {
			n := e.Name()
			if !strings.HasPrefix(n, "thermal_zone") {
				continue
			}
			milli := readIntFile(filepath.Join("/sys/class/thermal", n, "temp"))
			if milli > hottest {
				hottest = milli
			}
		}
		if hottest > 0 {
			st.CPUTempKnown = true
			st.CPUTempC = float64(hottest) / 1000.0
		}
	}

	return st
}

func readIntFile(path string) int {
	b, err := os.ReadFile(path)
	if err != nil {
		return -1
	}
	n, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil {
		return -1
	}
	return n
}

func readStringFile(path string) string {
	b, _ := os.ReadFile(path)
	return string(b)
}
