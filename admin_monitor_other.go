//go:build !linux && !windows

package main

func readProcessRSS() (uint64, bool) {
	return 0, false
}

func monitorCPUStats() (usage float64, load5, load15 *float64) {
	return 0, nil, nil
}

func monitorSystemMemory() (total, used, free uint64) {
	return 0, 0, 0
}
