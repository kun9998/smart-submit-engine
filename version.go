package main

import (
	"regexp"
	"strconv"
	"strings"
)

// 产品信息常量
const (
	ProductName    = "智能提交引擎"
	ProductNameEN  = "Smart Submit Engine"
	ProductVersion = "V3.5.1" // 当前版本号（内置在程序中）
)

var versionNumberRe = regexp.MustCompile(`(\d+)`)

// 获取版本号（直接返回内置版本号）
func getProductVersion() string {
	return ProductVersion
}

func parseVersionParts(v string) []int {
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(strings.TrimPrefix(v, "V"), "v")
	matches := versionNumberRe.FindAllString(v, -1)
	parts := make([]int, 0, len(matches))
	for _, m := range matches {
		n, _ := strconv.Atoi(m)
		parts = append(parts, n)
	}
	return parts
}

func compareVersions(a, b string) int {
	pa := parseVersionParts(a)
	pb := parseVersionParts(b)
	maxLen := len(pa)
	if len(pb) > maxLen {
		maxLen = len(pb)
	}
	for i := 0; i < maxLen; i++ {
		va, vb := 0, 0
		if i < len(pa) {
			va = pa[i]
		}
		if i < len(pb) {
			vb = pb[i]
		}
		if va < vb {
			return -1
		}
		if va > vb {
			return 1
		}
	}
	return 0
}

func isNewerVersion(current, latest string) bool {
	return compareVersions(current, latest) < 0
}

