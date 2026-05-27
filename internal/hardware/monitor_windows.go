//go:build windows

package hardware

import (
	"syscall"
	"unsafe"
)

func platformMonitor() Monitor { return &winMonitor{} }

type winMonitor struct{}

// SYSTEM_POWER_STATUS — see kernel32.dll GetSystemPowerStatus
//
//	BYTE  ACLineStatus       0 offline, 1 online, 255 unknown
//	BYTE  BatteryFlag        bitmask, 128 = no battery, 255 unknown
//	BYTE  BatteryLifePercent 0..100, 255 unknown
//	BYTE  SystemStatusFlag
//	DWORD BatteryLifeTime
//	DWORD BatteryFullLifeTime
type sysPowerStatus struct {
	ACLineStatus        byte
	BatteryFlag         byte
	BatteryLifePercent  byte
	SystemStatusFlag    byte
	BatteryLifeTime     uint32
	BatteryFullLifeTime uint32
}

var (
	kernel32                  = syscall.NewLazyDLL("kernel32.dll")
	procGetSystemPowerStatus  = kernel32.NewProc("GetSystemPowerStatus")
)

func (m *winMonitor) Read() Status {
	st := Status{Platform: "windows", Timestamp: nowIso(), BatteryPct: -1}

	var p sysPowerStatus
	ret, _, _ := procGetSystemPowerStatus.Call(uintptr(unsafe.Pointer(&p)))
	if ret != 0 {
		// 128 = "No system battery" — leave BatteryKnown false.
		if p.BatteryFlag != 128 && p.BatteryFlag != 255 {
			st.BatteryKnown = true
			if p.BatteryLifePercent != 255 {
				st.BatteryPct = int(p.BatteryLifePercent)
			} else {
				st.BatteryPct = -1
				st.BatteryKnown = false
			}
			// ACLineStatus 0 = offline (running on battery).
			st.OnBattery = p.ACLineStatus == 0
		}
	}

	// CPU temperature on Windows requires WMI which is non-trivial without
	// a third-party dep; we leave CPUTempKnown=false on this platform for v1.
	// Future work: read MSR_TEMPERATURE_TARGET via a driver, or call
	// MSAcpi_ThermalZoneTemperature over WMI.

	return st
}
