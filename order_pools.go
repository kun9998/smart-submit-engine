package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"
)

const (
	defaultSubmitPoolWorkers   = 64
	defaultSubmitPoolQueueCap  = 512
	defaultConfirmPoolWorkers  = 16
	defaultConfirmPoolQueueCap = 256
	submitPoolDispatchWait     = 50 * time.Millisecond
	confirmPoolDispatchWait    = 100 * time.Millisecond
)

var (
	submitPoolWorkers   = defaultSubmitPoolWorkers
	submitPoolQueueCap  = defaultSubmitPoolQueueCap
	confirmPoolWorkers  = defaultConfirmPoolWorkers
	confirmPoolQueueCap = defaultConfirmPoolQueueCap
)

type submitPoolJob struct {
	lock *DistributedLock
	hid  int
	msg  orderMsg
	val  string
	proc string
}

type confirmPoolJob struct {
	lock    *DistributedLock
	oid     int
	hid     int
	errmsg  string
	proc    string
	val     string
	callErr error
}

var (
	orderPoolsOnce         sync.Once
	orderPoolMu            sync.Mutex
	submitPoolJobs         chan submitPoolJob
	confirmPoolJobs        chan confirmPoolJob
	submitPoolActive       sync.WaitGroup
	confirmPoolActive      sync.WaitGroup
	submitWorkerCancels    []context.CancelFunc
	confirmWorkerCancels   []context.CancelFunc
	startedSubmitQueueCap  int
	startedConfirmQueueCap int
)

func clampPoolInt(v, min, max int) int {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

// applyPoolQueueGlobals 从全局队列配置更新 worker 池参数；并发数保存后热更新，队列长度需重启。
func applyPoolQueueGlobals(q *RuntimeQueueSection) {
	if q == nil {
		return
	}
	if q.SubmitPoolWorkers != nil && *q.SubmitPoolWorkers > 0 {
		submitPoolWorkers = clampPoolInt(*q.SubmitPoolWorkers, 1, 512)
	}
	if q.SubmitPoolQueueCap != nil && *q.SubmitPoolQueueCap > 0 {
		submitPoolQueueCap = clampPoolInt(*q.SubmitPoolQueueCap, 16, 4096)
	}
	if q.ConfirmPoolWorkers != nil && *q.ConfirmPoolWorkers > 0 {
		confirmPoolWorkers = clampPoolInt(*q.ConfirmPoolWorkers, 1, 128)
	}
	if q.ConfirmPoolQueueCap != nil && *q.ConfirmPoolQueueCap > 0 {
		confirmPoolQueueCap = clampPoolInt(*q.ConfirmPoolQueueCap, 16, 2048)
	}
	if orderQueueRootCtx != nil && orderQueueRootCtx.Err() == nil {
		reloadOrderWorkerPools(orderQueueRootCtx)
	}
}

func scaleSubmitPoolWorkers(parentCtx context.Context, n int) {
	for len(submitWorkerCancels) < n {
		wctx, cancel := context.WithCancel(parentCtx)
		submitWorkerCancels = append(submitWorkerCancels, cancel)
		go submitPoolWorker(wctx)
	}
	for len(submitWorkerCancels) > n {
		idx := len(submitWorkerCancels) - 1
		submitWorkerCancels[idx]()
		submitWorkerCancels = submitWorkerCancels[:idx]
	}
}

func scaleConfirmPoolWorkers(parentCtx context.Context, n int) {
	for len(confirmWorkerCancels) < n {
		wctx, cancel := context.WithCancel(parentCtx)
		confirmWorkerCancels = append(confirmWorkerCancels, cancel)
		go confirmPoolWorker(wctx)
	}
	for len(confirmWorkerCancels) > n {
		idx := len(confirmWorkerCancels) - 1
		confirmWorkerCancels[idx]()
		confirmWorkerCancels = confirmWorkerCancels[:idx]
	}
}

func reloadOrderWorkerPools(parentCtx context.Context) {
	orderPoolMu.Lock()
	defer orderPoolMu.Unlock()
	if submitPoolJobs == nil {
		return
	}
	if submitPoolQueueCap != startedSubmitQueueCap || confirmPoolQueueCap != startedConfirmQueueCap {
		log.Printf("提示: 任务排队上限已改，需重启程序后生效（提交 %d→%d，核对 %d→%d）；同时处理数量已按新配置更新",
			startedSubmitQueueCap, submitPoolQueueCap, startedConfirmQueueCap, confirmPoolQueueCap)
	}
	scaleSubmitPoolWorkers(parentCtx, submitPoolWorkers)
	scaleConfirmPoolWorkers(parentCtx, confirmPoolWorkers)
	log.Printf("订单处理线程已更新: 同时提交 %d 单，同时核对 %d 单", len(submitWorkerCancels), len(confirmWorkerCancels))
}

func startOrderWorkerPools(ctx context.Context) {
	orderPoolsOnce.Do(func() {
		orderPoolMu.Lock()
		submitPoolJobs = make(chan submitPoolJob, submitPoolQueueCap)
		confirmPoolJobs = make(chan confirmPoolJob, confirmPoolQueueCap)
		startedSubmitQueueCap = submitPoolQueueCap
		startedConfirmQueueCap = confirmPoolQueueCap
		scaleSubmitPoolWorkers(ctx, submitPoolWorkers)
		scaleConfirmPoolWorkers(ctx, confirmPoolWorkers)
		orderPoolMu.Unlock()
		log.Printf("订单处理线程已就绪: 同时提交 %d 单(排队上限 %d)，同时核对 %d 单(排队上限 %d)",
			submitPoolWorkers, submitPoolQueueCap, confirmPoolWorkers, confirmPoolQueueCap)
	})
}

func waitOrderWorkerPoolsIdle() {
	submitPoolActive.Wait()
	confirmPoolActive.Wait()
}

func tryDispatchSubmitPool(job submitPoolJob) bool {
	select {
	case submitPoolJobs <- job:
		return true
	default:
	}
	select {
	case submitPoolJobs <- job:
		return true
	case <-time.After(submitPoolDispatchWait):
		return false
	}
}

func tryDispatchConfirmPool(job confirmPoolJob) bool {
	select {
	case confirmPoolJobs <- job:
		return true
	default:
	}
	select {
	case confirmPoolJobs <- job:
		return true
	case <-time.After(confirmPoolDispatchWait):
		return false
	}
}

func submitPoolWorker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case job := <-submitPoolJobs:
			runSubmitPoolJob(job)
		}
	}
}

func confirmPoolWorker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case job := <-confirmPoolJobs:
			runSubmitTimeoutConfirmation(job)
		}
	}
}

func runSubmitPoolJob(job submitPoolJob) {
	submitPoolActive.Add(1)
	defer submitPoolActive.Done()

	handedOff := false
	defer func() {
		if !handedOff {
			_ = job.lock.Release(context.Background())
		}
	}()

	timeout := getSubmitTimeoutForHID(job.hid) + 10*time.Second
	submitCtx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	success, yid, errmsg, callErr := submitViaInternal(submitCtx, job.msg.OID)
	handedOff = finalizeSubmitOutcome(job.lock, job.hid, job.msg, job.val, job.proc, success, yid, errmsg, callErr)
}

// finalizeSubmitOutcome 处理提交结果。返回 true 表示锁已移交给后置确认池。
func finalizeSubmitOutcome(lock *DistributedLock, hid int, msg orderMsg, val, proc string, success bool, yid, errmsg string, callErr error) bool {
	ctx := context.Background()

	if success {
		_ = setSubmittedOrder(ctx, msg.OID, yid)
		_ = updateOrderStatusSubmitted(ctx, msg.OID, yid, hid)
		recordSubmitSuccess(hid)
		logSubmitOK(hid, msg.OID)
		_ = rdb.Del(ctx, enqKey(msg.OID)).Err()
		_ = rdb.LRem(ctx, proc, 1, val).Err()
		queueLenCacheMu.Lock()
		if _, ok := queueLenCache[hid]; ok {
			if queueLenCache[hid] > 0 {
				queueLenCache[hid]--
			}
			queueLenCacheTime[hid] = time.Now()
		}
		queueLenCacheMu.Unlock()
		return false
	}

	if callErr != nil {
		errmsg = SanitizeUserVisibleError(callErr.Error())
		lower := strings.ToLower(errmsg)
		isTimeout := errors.Is(callErr, ErrHTTPTimeout) ||
			strings.Contains(lower, "timeout") || strings.Contains(lower, "deadline exceeded") ||
			strings.Contains(lower, "connection reset") || strings.Contains(lower, "context canceled")
		if isTimeout {
			job := confirmPoolJob{
				lock: lock, oid: msg.OID, hid: hid, errmsg: errmsg,
				proc: proc, val: val, callErr: callErr,
			}
			if tryDispatchConfirmPool(job) {
				return true
			}
			runSubmitTimeoutConfirmation(job)
			return false
		}
	}

	isInvalidOidError := strings.Contains(errmsg, "缺少或非法的oid") ||
		strings.Contains(errmsg, "缺少或非法的 oid") ||
		strings.Contains(errmsg, "缺少或非法")
	if isInvalidOidError {
		time.Sleep(2 * time.Second)
		if ok, yidOk := orderAlreadySucceededInDB(ctx, msg.OID); ok {
			_ = setSubmittedOrder(ctx, msg.OID, yidOk)
			_ = updateOrderStatusSubmitted(ctx, msg.OID, yidOk, hid)
			recordSubmitSuccess(hid)
			logSubmitOKConfirmedDB(hid, msg.OID)
			_ = rdb.Del(ctx, enqKey(msg.OID)).Err()
			_ = rdb.LRem(ctx, proc, 1, val).Err()
			return false
		}
	}

	if tryHandleSubmitRetry(ctx, hid, msg, val, proc, errmsg, callErr) {
		return false
	}
	markOrderTerminalFailure(ctx, hid, msg, val, proc, errmsg, false)
	return false
}

func runSubmitTimeoutConfirmation(job confirmPoolJob) {
	confirmPoolActive.Add(1)
	defer confirmPoolActive.Done()
	defer func() { _ = job.lock.Release(context.Background()) }()

	confirmCtx, confirmCancel := context.WithTimeout(context.Background(), timeoutConfirmWait)
	defer confirmCancel()

	oid, hid, errmsg, proc, val, callErr := job.oid, job.hid, job.errmsg, job.proc, job.val, job.callErr

	if _, exists, err := getSubmittedOrder(confirmCtx, oid); err == nil && exists {
		_ = rdb.LRem(confirmCtx, proc, 1, val).Err()
		_ = rdb.Del(confirmCtx, enqKey(oid)).Err()
		return
	}
	ok, yidOk := orderAlreadySucceededInDB(confirmCtx, oid)
	if ok {
		_ = setSubmittedOrder(confirmCtx, oid, yidOk)
		_ = updateOrderStatusSubmitted(confirmCtx, oid, yidOk, hid)
		recordSubmitSuccess(hid)
		logSubmitOKConfirmedDB(hid, oid)
		_ = rdb.Del(confirmCtx, enqKey(oid)).Err()
		_ = rdb.LRem(confirmCtx, proc, 1, val).Err()
		return
	}
	var retryMsg orderMsg
	_ = json.Unmarshal([]byte(val), &retryMsg)
	cfg := getEffectiveResubmitForHID(hid)
	if cfg.Enabled && cfg.RetryOnTimeout && tryHandleSubmitRetry(confirmCtx, hid, retryMsg, val, proc, errmsg, callErr) {
		return
	}
	name := submitLogChannel(hid)
	logSubmitTimeout(hid, oid, errmsg)
	simplifiedErrmsg := simplifyErrorMsg(errmsg)
	orderTable := tableName("order")
	_, _ = db.ExecContext(confirmCtx,
		fmt.Sprintf(`UPDATE %s SET status='提交异常', remarks=?, dockstatus='2' WHERE oid=? LIMIT 1`, orderTable),
		fmt.Sprintf("提交超时：%s", simplifiedErrmsg), oid,
	)
	if alertShowdocURL != "" {
		title := fmt.Sprintf("订单提交超时 · %s", name)
		content := fmt.Sprintf("订单号：%d\n渠道：%s\n原因：%s\n已标记为提交异常，请到管理端查看", oid, name, SanitizeUserVisibleError(errmsg))
		go sendNotification(notifySubmitTimeout, title, content)
	}
	atomicAddDLQ(hid)
	_ = rdb.LRem(confirmCtx, proc, 1, val).Err()
	b, _ := json.Marshal(retryMsg)
	_ = rdb.LPush(confirmCtx, dlqKey(hid), string(b)).Err()
	_ = rdb.Del(confirmCtx, enqKey(oid)).Err()
}
