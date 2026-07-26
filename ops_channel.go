package main

import (
	"context"
	"fmt"
	"sync"
	"time"
)

var (
	orderQueueRootCtx context.Context
	orderQueueWorkerWG sync.WaitGroup

	opsPausedMu sync.RWMutex
	opsPausedAt = map[int]time.Time{}
)

func setOrderQueueRootCtx(ctx context.Context) {
	orderQueueRootCtx = ctx
}

func opsIsChannelPaused(hid int) bool {
	opsPausedMu.RLock()
	defer opsPausedMu.RUnlock()
	_, ok := opsPausedAt[hid]
	return ok
}

func listPausedChannelHIDs() []int {
	opsPausedMu.RLock()
	defer opsPausedMu.RUnlock()
	out := make([]int, 0, len(opsPausedAt))
	for hid := range opsPausedAt {
		out = append(out, hid)
	}
	return out
}

func pausedChannelSet() map[int]bool {
	opsPausedMu.RLock()
	defer opsPausedMu.RUnlock()
	out := make(map[int]bool, len(opsPausedAt))
	for hid := range opsPausedAt {
		out[hid] = true
	}
	return out
}

func pausedChannelDTOs() []map[string]interface{} {
	opsPausedMu.RLock()
	defer opsPausedMu.RUnlock()
	out := make([]map[string]interface{}, 0, len(opsPausedAt))
	for hid, since := range opsPausedAt {
		name := getHuoyuanName(hid)
		if name == "" {
			name = fmt.Sprintf("hid%d", hid)
		}
		out = append(out, map[string]interface{}{
			"hid":   hid,
			"name":  name,
			"since": since.Format(time.RFC3339),
		})
	}
	return out
}

func channelExistsInEngine(hid int) bool {
	hidsMu.RLock()
	defer hidsMu.RUnlock()
	for _, h := range hids {
		if h == hid {
			return true
		}
	}
	return false
}

func opsPauseChannel(hid int) error {
	if !OrderEngineReady() || !orderQueueStarted {
		return fmt.Errorf("订单引擎未运行")
	}
	if !channelExistsInEngine(hid) {
		return fmt.Errorf("HID %d 不存在或未激活", hid)
	}
	opsPausedMu.Lock()
	if _, ok := opsPausedAt[hid]; ok {
		opsPausedMu.Unlock()
		return nil
	}
	opsPausedAt[hid] = time.Now()
	opsPausedMu.Unlock()

	concurrencyMu.Lock()
	lst := workerCancels[hid]
	for _, cancel := range lst {
		cancel()
	}
	workerCancels[hid] = nil
	currWorkers[hid] = 0
	concurrencyMu.Unlock()
	resetOpsResumeStable(hid)
	return nil
}

func opsResumeChannel(hid int) error {
	if !OrderEngineReady() || !orderQueueStarted {
		return fmt.Errorf("订单引擎未运行")
	}
	if orderQueueRootCtx == nil || orderQueueRootCtx.Err() != nil {
		return fmt.Errorf("订单队列上下文不可用")
	}
	if !channelExistsInEngine(hid) {
		return fmt.Errorf("HID %d 不存在或未激活", hid)
	}

	opsPausedMu.Lock()
	if _, ok := opsPausedAt[hid]; !ok {
		opsPausedMu.Unlock()
		return fmt.Errorf("HID %d 未处于暂停状态", hid)
	}
	delete(opsPausedAt, hid)
	opsPausedMu.Unlock()

	qCfg := getEffectiveQueueForHID(hid)
	n := qCfg.MinWorkersPerHID
	if n <= 0 {
		n = minWorkersPerHID
	}

	concurrencyMu.Lock()
	defer concurrencyMu.Unlock()
	for i := 0; i < n; i++ {
		wctx, cancel := context.WithCancel(orderQueueRootCtx)
		orderQueueWorkerWG.Add(1)
		go consumer(wctx, hid, &orderQueueWorkerWG)
		workerCancels[hid] = append(workerCancels[hid], cancel)
		currWorkers[hid]++
	}
	return nil
}

func opsAdjustWorkers(hid int, delta int) error {
	if opsIsChannelPaused(hid) {
		return fmt.Errorf("HID %d 已暂停，无法调整 worker", hid)
	}
	if !OrderEngineReady() || !orderQueueStarted || orderQueueRootCtx == nil {
		return fmt.Errorf("订单引擎未运行")
	}
	if delta == 0 {
		return nil
	}
	if delta > 4 {
		delta = 4
	}
	if delta < -4 {
		delta = -4
	}

	qCfg := getEffectiveQueueForHID(hid)
	concurrencyMu.Lock()
	defer concurrencyMu.Unlock()
	curr := currWorkers[hid]

	if delta > 0 {
		maxW := qCfg.MaxWorkersPerHID
		if maxW <= 0 {
			maxW = maxWorkersPerHID
		}
		for i := 0; i < delta && curr < maxW; i++ {
			wctx, cancel := context.WithCancel(orderQueueRootCtx)
			orderQueueWorkerWG.Add(1)
			go consumer(wctx, hid, &orderQueueWorkerWG)
			workerCancels[hid] = append(workerCancels[hid], cancel)
			curr++
		}
	} else {
		for i := 0; i < -delta && curr > qCfg.MinWorkersPerHID; i++ {
			lst := workerCancels[hid]
			if len(lst) == 0 {
				break
			}
			cancel := lst[len(lst)-1]
			workerCancels[hid] = lst[:len(lst)-1]
			cancel()
			curr--
		}
	}
	currWorkers[hid] = curr
	return nil
}
