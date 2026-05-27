//go:build darwin

package hardware

import (
	"os/exec"
	"strconv"
	"strings"
)

func platformMonitor() Monitor { return &darwinMonitor{} }

type darwinMonitor struct{}

func (m *darwinMonitor) Read() Status {
	st := Status{Platform: "darwin", Timestamp: nowIso(), BatteryPct: -1}

	// `pmset -g batt` outputs lines like:
	//   Now drawing from 'Battery Power'
	//    -InternalBattery-0 (id=…) 82%; discharging; …
	out, err := exec.Command("pmset", "-g", "batt").Output()
	if err == nil {
		text := string(out)
		if strings.Contains(text, "Battery Power") {
			st.OnBattery = true
		} else if strings.Contains(text, "AC Power") {
			st.OnBattery = false
		}
		// Find "NN%;"
		if i := strings.Index(text, "%;"); i >= 0 {
			start := i
			for start > 0 && text[start-1] >= '0' && text[start-1] <= '9' {
				start--
			}
			if pct, err := strconv.Atoi(text[start:i]); err == nil {
				st.BatteryKnown = true
				st.BatteryPct = pct
			}
		}
	}

	// CPU temperature via `powermetrics` requires root; skip on v1 and
	// leave CPUTempKnown=false. Future work: SMC keys via IOKit (CGO).

	return st
}
