package main

import (
	"fmt"
	"time"
)

func submitLogChannel(hid int) string {
	name := getHuoyuanName(hid)
	if name == "" {
		return fmt.Sprintf("渠道号 %d", hid)
	}
	return name
}

func logSubmitOK(hid, oid int) {
	pushSubmitLog("success", "订单提交成功 · %s · 订单号 %d", submitLogChannel(hid), oid)
}

func logSubmitOKConfirmedDB(hid, oid int) {
	pushSubmitLog("success", "订单提交成功 · %s · 订单号 %d · 查数据库后确认", submitLogChannel(hid), oid)
}

func logSubmitOKAutoVerified(hid, oid int) {
	pushSubmitLog("success", "订单提交成功 · %s · 订单号 %d · 系统自动核对后确认", submitLogChannel(hid), oid)
}

func logSubmitFail(hid, oid int, reason string) {
	pushSubmitLog("error", "订单提交失败 · %s · 订单号 %d · 原因：%s", submitLogChannel(hid), oid, SanitizeUserVisibleError(reason))
}

func logSubmitTimeout(hid, oid int, reason string) {
	pushSubmitLog("error", "订单提交超时 · %s · 订单号 %d · 原因：%s", submitLogChannel(hid), oid, SanitizeUserVisibleError(reason))
}

func logSubmitDBFail(hid, oid int) {
	pushSubmitLog("error", "更新订单状态失败 · %s · 订单号 %d", submitLogChannel(hid), oid)
}

func logSubmitProcessTooLong(hid, oid int) {
	pushSubmitLog("error", "订单处理太久 · %s · 订单号 %d · 已标记为提交异常", submitLogChannel(hid), oid)
}

func logSubmitRateLimited(hid, oid int) {
	pushSubmitLog("warn", "提交太快被限流 · %s · 订单号 %d · 稍后会自动重试", submitLogChannel(hid), oid)
}

func logSubmitRetryLater(hid, oid, attempt int, delay time.Duration) {
	pushSubmitLog("warn", "稍后会自动重试 · %s · 订单号 %d · 第 %d 次 · %s 后再试",
		submitLogChannel(hid), oid, attempt, delay.Round(time.Second))
}

func logSubmitRequeued(hid, oid int) {
	pushSubmitLog("info", "失败订单重新排队 · %s · 订单号 %d", submitLogChannel(hid), oid)
}
