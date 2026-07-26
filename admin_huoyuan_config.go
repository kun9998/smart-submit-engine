package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
)

type huoyuanListItemDTO struct {
	HID        int    `json:"hid"`
	Name       string `json:"name"`
	Platform   string `json:"pt"`
	URL        string `json:"url"`
	HasConfig  bool   `json:"has_config"`
	Remark     string `json:"remark,omitempty"`
}

type runtimeConfigViewDTO struct {
	Defaults   *RuntimeConfigPayload `json:"defaults"`
	Override   *RuntimeConfigPayload `json:"override,omitempty"`
	Effective  *RuntimeConfigPayload `json:"effective"`
	OpsDefaults OpsConfig            `json:"ops_defaults"`
	Ops         OpsConfig            `json:"ops"`
}

func buildRuntimeConfigViewDTO() runtimeConfigViewDTO {
	defaults := defaultRuntimeConfigBase()
	override := getRuntimeGlobalOverride()
	effective := mergeRuntimeConfig(&defaults, override)
	return runtimeConfigViewDTO{
		Defaults:    &defaults,
		Override:    override,
		Effective:   effective,
		OpsDefaults: defaultOpsConfig(),
		Ops:         getOpsConfig(),
	}
}

func adminHuoyuanListHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"code": -1, "msg": "方法不允许"})
		return
	}
	if db == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{"code": -1, "msg": "主站数据库未连接"})
		return
	}
	ctx := r.Context()
	hidCfg := listRuntimeHIDWithConfig()
	q := fmt.Sprintf(`SELECT hid, name, pt, url FROM %s ORDER BY hid ASC`, tableName("huoyuan"))
	rows, err := db.QueryContext(ctx, q)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"code": -1, "msg": err.Error()})
		return
	}
	defer rows.Close()

	list := make([]huoyuanListItemDTO, 0)
	for rows.Next() {
		var item huoyuanListItemDTO
		if err := rows.Scan(&item.HID, &item.Name, &item.Platform, &item.URL); err != nil {
			continue
		}
		item.HasConfig = hidCfg[item.HID]
		if item.HasConfig {
			if ov := getRuntimeHIDOverride(item.HID); ov != nil {
				_ = ov
			}
			if pluginDB != nil {
				var remark string
				rq := fmt.Sprintf(`SELECT remark FROM %s WHERE hid=? LIMIT 1`, pluginTable("huoyuan_runtime"))
				_ = pluginDB.QueryRowContext(ctx, rq, item.HID).Scan(&remark)
				item.Remark = remark
			}
		}
		list = append(list, item)
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"code": 1, "msg": "ok", "data": list})
}

func adminHuoyuanConfigGlobalHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"code": 1,
			"msg":  "ok",
			"data": buildRuntimeConfigViewDTO(),
		})
	case http.MethodPut:
		body, err := io.ReadAll(r.Body)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]interface{}{"code": -1, "msg": "读取请求失败"})
			return
		}
		var req struct {
			Config *RuntimeConfigPayload `json:"config"`
			Ops    *OpsConfig            `json:"ops,omitempty"`
		}
		if err := json.Unmarshal(body, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]interface{}{"code": -1, "msg": "JSON 无效"})
			return
		}
		if req.Ops != nil {
			if err := saveOpsConfig(ctx, *req.Ops); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"code": -1, "msg": err.Error()})
				return
			}
		}
		if req.Config == nil || runtimePayloadEmpty(req.Config) {
			if err := deleteRuntimeGlobalConfig(ctx); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"code": -1, "msg": err.Error()})
				return
			}
		} else if err := saveRuntimeGlobalConfig(ctx, req.Config); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"code": -1, "msg": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"code": 1,
			"msg":  "已保存",
			"data": buildRuntimeConfigViewDTO(),
		})
	case http.MethodDelete:
		if err := deleteRuntimeGlobalConfig(ctx); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"code": -1, "msg": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"code": 1, "msg": "已恢复为系统默认"})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"code": -1, "msg": "方法不允许"})
	}
}

func adminHuoyuanConfigByHIDHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	path := strings.TrimPrefix(r.URL.Path, "/api/huoyuan-config/hid/")
	hidStr := strings.Trim(strings.TrimSpace(path), "/")
	hid, err := strconv.Atoi(hidStr)
	if err != nil || hid <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"code": -1, "msg": "hid 无效"})
		return
	}

	switch r.Method {
	case http.MethodGet:
		defaults := defaultRuntimeConfigBase()
		global := getRuntimeGlobalOverride()
		base := mergeRuntimeConfig(&defaults, global)
		override := getRuntimeHIDOverride(hid)
		effective := mergeRuntimeConfig(base, override)
		if override == nil {
			override = &RuntimeConfigPayload{}
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"code": 1,
			"msg":  "ok",
			"data": map[string]interface{}{
				"hid":       hid,
				"defaults":  defaults,
				"override":  override,
				"effective": effective,
			},
		})
	case http.MethodPut:
		body, err := io.ReadAll(r.Body)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]interface{}{"code": -1, "msg": "读取请求失败"})
			return
		}
		var req struct {
			Config *RuntimeConfigPayload `json:"config"`
			Remark string                `json:"remark"`
		}
		if err := json.Unmarshal(body, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]interface{}{"code": -1, "msg": "JSON 无效"})
			return
		}
		if req.Config == nil || runtimePayloadEmpty(req.Config) {
			if strings.TrimSpace(req.Remark) == "" {
				if err := deleteRuntimeHIDConfig(ctx, hid); err != nil {
					writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"code": -1, "msg": err.Error()})
					return
				}
			} else if err := saveRuntimeHIDConfig(ctx, hid, &RuntimeConfigPayload{}, req.Remark); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"code": -1, "msg": err.Error()})
				return
			}
		} else if err := saveRuntimeHIDConfig(ctx, hid, req.Config, req.Remark); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"code": -1, "msg": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"code": 1, "msg": "已保存"})
	case http.MethodDelete:
		if err := deleteRuntimeHIDConfig(ctx, hid); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"code": -1, "msg": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"code": 1, "msg": "已删除单独配置"})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"code": -1, "msg": "方法不允许"})
	}
}
