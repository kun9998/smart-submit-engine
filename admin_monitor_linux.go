//go:build linux

package main

import (
	"bufio"
	"os"
	"strconv"
	"strings"
	"time"
)

func readProcessRSS() (uint64, bool) {
	return readProcessRSSLinux()
}

func monitorCPUStats() (usage float64, load5, load15 *float64) {
	usage = readLinuxCPUPercent()
	l5, l15, ok := readLinuxLoadAvg()
	if ok {
		load5 = &l5
		load15 = &l15
	}
	return usage, load5, load15
}

func monitorSystemMemory() (total, used, free uint64) {
	return readLinuxMemory()
}

func readLinuxCPUPercent() float64 {
	idle1, total1, ok1 := readProcStatAggregates()
	if !ok1 {
		return 0
	}
	time.Sleep(200 * time.Millisecond)
	idle2, total2, ok2 := readProcStatAggregates()
	if !ok2 || total2 <= total1 {
		return 0
	}
	idleDelta := float64(idle2 - idle1)
	totalDelta := float64(total2 - total1)
	if totalDelta <= 0 {
		return 0
	}
	return (1 - idleDelta/totalDelta) * 100
}

func readProcStatAggregates() (idle, total uint64, ok bool) {
	f, err := os.Open("/proc/stat")
	if err != nil {
		return 0, 0, false
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	if !sc.Scan() {
		return 0, 0, false
	}
	fields := strings.Fields(sc.Text())
	if len(fields) < 5 || fields[0] != "cpu" {
		return 0, 0, false
	}
	vals := make([]uint64, 0, len(fields)-1)
	for _, s := range fields[1:] {
		n, err := strconv.ParseUint(s, 10, 64)
		if err != nil {
			return 0, 0, false
		}
		vals = append(vals, n)
	}
	for _, n := range vals {
		total += n
	}
	if len(vals) >= 4 {
		idle = vals[3]
		if len(vals) > 4 {
			idle += vals[4]
		}
	}
	return idle, total, true
}

func readLinuxLoadAvg() (load5, load15 float64, ok bool) {
	b, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return 0, 0, false
	}
	fields := strings.Fields(string(b))
	if len(fields) < 3 {
		return 0, 0, false
	}
	l5, err1 := strconv.ParseFloat(fields[1], 64)
	l15, err2 := strconv.ParseFloat(fields[2], 64)
	if err1 != nil || err2 != nil {
		return 0, 0, false
	}
	return l5, l15, true
}

func readLinuxMemory() (total, used, free uint64) {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0, 0, 0
	}
	defer f.Close()

	var memTotal, memAvailable, memFree uint64
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		val, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			continue
		}
		val *= 1024
		switch fields[0] {
		case "MemTotal:":
			memTotal = val
		case "MemAvailable:":
			memAvailable = val
		case "MemFree:":
			memFree = val
		}
	}
	if memTotal == 0 {
		return 0, 0, 0
	}
	if memAvailable > 0 {
		free = memAvailable
	} else if memFree > 0 {
		free = memFree
	}
	if free > memTotal {
		free = memFree
	}
	used = memTotal - free
	return memTotal, used, free
}
