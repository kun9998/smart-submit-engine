package main

import (
	"strconv"
	"sync"
	"time"
)

type opsFailRateSample struct {
	At   time.Time
	Rate float64
	Fail uint64
}

var (
	opsFailRateHistMu sync.Mutex
	opsFailRateHist   = map[int][]opsFailRateSample{}
)

const opsFailRateHistoryRetention = 15 * time.Minute
const opsFailRateSpikeLookback = 5 * time.Minute

func recordOpsFailRateSamples(channels []opsChannelContextDTO) {
	now := time.Now()
	opsFailRateHistMu.Lock()
	defer opsFailRateHistMu.Unlock()
	cutoff := now.Add(-opsFailRateHistoryRetention)
	for _, ch := range channels {
		samples := append(opsFailRateHist[ch.HID], opsFailRateSample{
			At:   now,
			Rate: ch.FailRatePct,
			Fail: ch.WindowFail,
		})
		kept := samples[:0]
		for _, s := range samples {
			if s.At.After(cutoff) {
				kept = append(kept, s)
			}
		}
		opsFailRateHist[ch.HID] = kept
	}
}

func detectChannelFailRateSpikes(channels []opsChannelContextDTO, thresholdPP float64) []string {
	if thresholdPP <= 0 {
		thresholdPP = 15
	}
	events := make([]string, 0)
	opsFailRateHistMu.Lock()
	defer opsFailRateHistMu.Unlock()
	now := time.Now()
	lookback := now.Add(-opsFailRateSpikeLookback)

	for _, ch := range channels {
		if ch.WindowFail < 5 {
			continue
		}
		samples := opsFailRateHist[ch.HID]
		if len(samples) < 2 {
			continue
		}
		current := samples[len(samples)-1]
		var baseline *opsFailRateSample
		for i := len(samples) - 2; i >= 0; i-- {
			s := samples[i]
			if !s.At.After(lookback) {
				baseline = &s
				break
			}
		}
		if baseline == nil {
			continue
		}
		delta := current.Rate - baseline.Rate
		if delta >= thresholdPP {
			events = append(events, "channel_fail_rate_spike:"+strconv.Itoa(ch.HID))
		}
	}
	return events
}

func resetOpsFailRateHistoryForHID(hid int) {
	opsFailRateHistMu.Lock()
	delete(opsFailRateHist, hid)
	opsFailRateHistMu.Unlock()
}
