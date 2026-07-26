package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
)

func executeStarlarkSubmitRule(ctx context.Context, order *Order, hy *Huoyuan, httpClient *http.Client, rule SubmitRuleConfig, src string, timeoutMS int) (*AddOrderResult, error) {
	v, err := runStarlarkScript(ctx, order, hy, httpClient, rule, src, timeoutMS)
	if err != nil {
		return &AddOrderResult{Code: -1, Msg: err.Error()}, nil
	}
	return starlarkValueToAddOrder(v)
}

func executeStarlarkProcessRule(ctx context.Context, order *Order, hy *Huoyuan, httpClient *http.Client, proc ProcessRuleConfig, src string, timeoutMS int) ([]*ProcessCxResult, error) {
	base := submitRuleFromProcess(proc)
	v, err := runStarlarkScript(ctx, order, hy, httpClient, base, src, timeoutMS)
	if err != nil {
		return nil, err
	}
	return starlarkValueToProcessResults(v)
}

func runStarlarkScript(ctx context.Context, order *Order, hy *Huoyuan, httpClient *http.Client, rule SubmitRuleConfig, src string, timeoutMS int) (starlark.Value, error) {
	if timeoutMS <= 0 {
		timeoutMS = 60000
	}
	if timeoutMS > 120000 {
		timeoutMS = 120000
	}
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(timeoutMS)*time.Millisecond)
		defer cancel()
	}

	client := pluginHTTPClient(httpClient)
	tctx := newSubmitTemplateCtx(rule)
	thread := &starlark.Thread{Name: "submit"}
	thread.SetMaxExecutionSteps(1_000_000)

	predeclared := starlark.StringDict{
		"order":   starlarkOrderDict(order),
		"huoyuan": starlarkHuoyuanDict(hy),
		"render":  starlarkNewRenderBuiltin(order, hy, tctx),
		"http":    starlarkHTTPModule(ctx, client, order, hy, tctx),
	}

	globals, err := starlark.ExecFile(thread, "submit", src, predeclared)
	if err != nil {
		return nil, fmt.Errorf("Starlark 脚本错误: %w", err)
	}
	v, ok := globals["result"]
	if !ok || v == nil || v == starlark.None {
		return nil, fmt.Errorf("脚本未设置 result")
	}
	return v, nil
}

func starlarkOrderDict(order *Order) *starlarkstruct.Struct {
	if order == nil {
		return &starlarkstruct.Struct{}
	}
	return starlarkstruct.FromStringDict(starlarkstruct.Default, starlark.StringDict{
		"oid":                starlark.String(order.OID),
		"hid":                starlark.String(order.HID),
		"user":               starlark.String(order.User),
		"pass":               starlark.String(order.Pass),
		"kcname":             starlark.String(order.KCName),
		"status":             starlark.String(order.Status),
		"process":            starlark.String(order.Process),
		"remarks":            starlark.String(order.Remarks),
		"yid":                starlark.String(order.YID),
		"school":             starlark.String(order.School),
		"noun":               starlark.String(order.Noun),
		"kcid":               starlark.String(order.KCID),
		"name":               starlark.String(order.Name),
		"ikun_study_ip":      starlark.String(order.IkunStudyIP),
		"simple_day_score":   starlark.String(order.SimpleDayScore),
		"simple_total_score": starlark.String(order.SimpleTotalScore),
		"simple_learn_time":  starlark.String(order.SimpleLearnTime),
	})
}

func starlarkHuoyuanDict(hy *Huoyuan) *starlarkstruct.Struct {
	if hy == nil {
		return &starlarkstruct.Struct{}
	}
	return starlarkstruct.FromStringDict(starlarkstruct.Default, starlark.StringDict{
		"url":    starlark.String(hy.URL),
		"user":   starlark.String(hy.User),
		"pass":   starlark.String(hy.Pass),
		"token":  starlark.String(hy.Token),
		"cookie": starlark.String(hy.Cookie),
	})
}

func starlarkNewRenderBuiltin(order *Order, hy *Huoyuan, tctx *submitTemplateCtx) starlark.Callable {
	return starlark.NewBuiltin("render", func(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
		if args.Len() != 1 {
			return nil, fmt.Errorf("render: want 1 argument")
		}
		tpl, ok := starlark.AsString(args.Index(0))
		if !ok {
			return nil, fmt.Errorf("render: argument must be string")
		}
		out, err := renderSubmitTemplate(tpl, order, hy, tctx)
		if err != nil {
			return nil, err
		}
		return starlark.String(out), nil
	})
}

func starlarkHTTPModule(ctx context.Context, client *http.Client, order *Order, hy *Huoyuan, tctx *submitTemplateCtx) *starlarkstruct.Module {
	return &starlarkstruct.Module{
		Name: "http",
		Members: starlark.StringDict{
			"get":        starlarkNewHTTPBuiltin(ctx, client, order, hy, tctx, "GET"),
			"post":       starlarkNewHTTPBuiltin(ctx, client, order, hy, tctx, "POST"),
			"post_json":  starlarkNewHTTPJSONBuiltin(ctx, client, order, hy, tctx),
			"request":    starlarkNewHTTPRequestBuiltin(ctx, client, order, hy, tctx),
		},
	}
}

func starlarkNewHTTPBuiltin(ctx context.Context, client *http.Client, order *Order, hy *Huoyuan, tctx *submitTemplateCtx, method string) starlark.Callable {
	return starlark.NewBuiltin(strings.ToLower(method), func(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		if args.Len() < 1 {
			return nil, fmt.Errorf("http.%s: want url", strings.ToLower(method))
		}
		urlTpl, ok := starlark.AsString(args.Index(0))
		if !ok {
			return nil, fmt.Errorf("http.%s: url must be string", strings.ToLower(method))
		}
		bodyMap, hdrMap := starlarkReadBodyHeaders(args, kwargs, 1)
		return starlarkDoHTTP(ctx, client, order, hy, tctx, method, urlTpl, bodyMap, hdrMap)
	})
}

func starlarkNewHTTPJSONBuiltin(ctx context.Context, client *http.Client, order *Order, hy *Huoyuan, tctx *submitTemplateCtx) starlark.Callable {
	return starlark.NewBuiltin("post_json", func(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		if args.Len() < 1 {
			return nil, fmt.Errorf("http.post_json: want url")
		}
		urlTpl, ok := starlark.AsString(args.Index(0))
		if !ok {
			return nil, fmt.Errorf("http.post_json: url must be string")
		}
		bodyMap, hdrMap := starlarkReadBodyHeaders(args, kwargs, 1)
		if hdrMap == nil {
			hdrMap = map[string]string{}
		}
		hdrMap["Content-Type"] = "application/json;charset=utf-8"
		rule := SubmitRuleConfig{
			Method:      "POST",
			URL:         urlTpl,
			ContentType: "json",
			Headers:     hdrMap,
			Body:        bodyMap,
		}
		body, err := executeRuleHTTPRequest(ctx, client, order, hy, tctx, rule)
		if err != nil {
			return nil, err
		}
		return jsonStringToStarlark(body)
	})
}

func starlarkNewHTTPRequestBuiltin(ctx context.Context, client *http.Client, order *Order, hy *Huoyuan, tctx *submitTemplateCtx) starlark.Callable {
	return starlark.NewBuiltin("request", func(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		method := "GET"
		if args.Len() >= 1 {
			if m, ok := starlark.AsString(args.Index(0)); ok {
				method = strings.ToUpper(m)
			}
		}
		if args.Len() < 2 {
			return nil, fmt.Errorf("http.request: want method, url")
		}
		urlTpl, ok := starlark.AsString(args.Index(1))
		if !ok {
			return nil, fmt.Errorf("http.request: url must be string")
		}
		bodyMap, hdrMap := starlarkReadBodyHeaders(args, kwargs, 2)
		return starlarkDoHTTP(ctx, client, order, hy, tctx, method, urlTpl, bodyMap, hdrMap)
	})
}

func starlarkReadBodyHeaders(args starlark.Tuple, kwargs []starlark.Tuple, urlArgCount int) (map[string]string, map[string]string) {
	var body map[string]string
	var hdr map[string]string
	if args.Len() > urlArgCount {
		if m, ok := args.Index(urlArgCount).(*starlark.Dict); ok {
			body = starlarkDictToStringMap(m)
		}
	}
	for _, kw := range kwargs {
		key, ok := starlark.AsString(kw.Index(0))
		if !ok {
			continue
		}
		switch key {
		case "body":
			if m, ok := kw.Index(1).(*starlark.Dict); ok {
				body = starlarkDictToStringMap(m)
			}
		case "headers":
			if m, ok := kw.Index(1).(*starlark.Dict); ok {
				hdr = starlarkDictToStringMap(m)
			}
		}
	}
	return body, hdr
}

func starlarkDoHTTP(ctx context.Context, client *http.Client, order *Order, hy *Huoyuan, tctx *submitTemplateCtx, method, urlTpl string, body map[string]string, hdr map[string]string) (starlark.Value, error) {
	if body == nil {
		body = map[string]string{}
	}
	if hdr == nil {
		hdr = map[string]string{}
	}
	contentType := "form"
	if ct := hdr["Content-Type"]; strings.Contains(strings.ToLower(ct), "json") {
		contentType = "json"
	}
	rule := SubmitRuleConfig{
		Method:      method,
		URL:         urlTpl,
		ContentType: contentType,
		Headers:     hdr,
		Body:        body,
	}
	respBody, err := executeRuleHTTPRequest(ctx, client, order, hy, tctx, rule)
	if err != nil {
		return nil, err
	}
	return jsonStringToStarlark(respBody)
}

func starlarkDictToStringMap(d *starlark.Dict) map[string]string {
	out := map[string]string{}
	if d == nil {
		return out
	}
	for _, item := range d.Items() {
		k, ok := starlark.AsString(item[0])
		if !ok {
			continue
		}
		out[k] = flexString(starlarkToGo(item[1]))
	}
	return out
}

func starlarkToGo(v starlark.Value) interface{} {
	switch x := v.(type) {
	case starlark.String:
		return string(x)
	case starlark.Int:
		i, _ := x.Int64()
		return i
	case starlark.Float:
		return float64(x)
	case starlark.Bool:
		return bool(x)
	case *starlark.Dict:
		m := map[string]interface{}{}
		for _, item := range x.Items() {
			if k, ok := starlark.AsString(item[0]); ok {
				m[k] = starlarkToGo(item[1])
			}
		}
		return m
	case *starlark.List:
		arr := make([]interface{}, 0, x.Len())
		for i := 0; i < x.Len(); i++ {
			v := x.Index(i)
			arr = append(arr, starlarkToGo(v))
		}
		return arr
	default:
		return fmt.Sprint(v)
	}
}

func jsonStringToStarlark(body string) (starlark.Value, error) {
	body = strings.TrimSpace(body)
	if body == "" {
		return starlark.None, nil
	}
	var anyRoot interface{}
	if err := json.Unmarshal([]byte(body), &anyRoot); err != nil {
		return starlark.String(body), nil
	}
	return goToStarlark(anyRoot)
}

func goToStarlark(v interface{}) (starlark.Value, error) {
	switch x := v.(type) {
	case nil:
		return starlark.None, nil
	case bool:
		return starlark.Bool(x), nil
	case float64:
		if x == float64(int64(x)) {
			return starlark.MakeInt64(int64(x)), nil
		}
		return starlark.Float(x), nil
	case string:
		return starlark.String(x), nil
	case []interface{}:
	 elems := make([]starlark.Value, 0, len(x))
		for _, it := range x {
			sv, err := goToStarlark(it)
			if err != nil {
				return nil, err
			}
			elems = append(elems, sv)
		}
		return starlark.NewList(elems), nil
	case map[string]interface{}:
		d := starlark.NewDict(len(x))
		for k, it := range x {
			sv, err := goToStarlark(it)
			if err != nil {
				return nil, err
			}
			if err := d.SetKey(starlark.String(k), sv); err != nil {
				return nil, err
			}
		}
		return d, nil
	default:
		return starlark.String(flexString(x)), nil
	}
}

func starlarkValueToAddOrder(v starlark.Value) (*AddOrderResult, error) {
	d, ok := v.(*starlark.Dict)
	if !ok {
		return nil, fmt.Errorf("result 必须是 dict")
	}
	get := func(key string) string {
		val, found, _ := d.Get(starlark.String(key))
		if !found {
			return ""
		}
		s, _ := starlark.AsString(val)
		if s != "" {
			return s
		}
		return flexString(starlarkToGo(val))
	}
	code := -1
	if val, found, _ := d.Get(starlark.String("code")); found {
		switch x := val.(type) {
		case starlark.Int:
			i, _ := x.Int64()
			code = int(i)
		case starlark.String:
			code = flexInt(string(x), -1)
		case starlark.Bool:
			if x {
				code = 1
			}
		}
	}
	return &AddOrderResult{
		Code: code,
		Msg:  firstNonEmpty(get("msg"), "完成"),
		YID:  get("yid"),
	}, nil
}

func starlarkValueToProcessResults(v starlark.Value) ([]*ProcessCxResult, error) {
	switch x := v.(type) {
	case *starlark.List:
		out := make([]*ProcessCxResult, 0, x.Len())
		for i := 0; i < x.Len(); i++ {
			item := x.Index(i)
			one, err := starlarkDictToProcessResult(item)
			if err != nil {
				return nil, err
			}
			out = append(out, one)
		}
		return out, nil
	default:
		one, err := starlarkDictToProcessResult(v)
		if err != nil {
			return nil, err
		}
		return []*ProcessCxResult{one}, nil
	}
}

func starlarkDictToProcessResult(v starlark.Value) (*ProcessCxResult, error) {
	d, ok := v.(*starlark.Dict)
	if !ok {
		return nil, fmt.Errorf("process result 必须是 dict 或 list[dict]")
	}
	get := func(key string) string {
		val, found, _ := d.Get(starlark.String(key))
		if !found {
			return ""
		}
		s, ok := starlark.AsString(val)
		if ok {
			return s
		}
		return flexString(starlarkToGo(val))
	}
	code := 1
	if val, found, _ := d.Get(starlark.String("code")); found {
		switch x := val.(type) {
		case starlark.Int:
			i, _ := x.Int64()
			code = int(i)
		case starlark.String:
			code = flexInt(string(x), 1)
		}
	}
	return &ProcessCxResult{
		Code:       code,
		Msg:        firstNonEmpty(get("msg"), "ok"),
		YID:        get("yid"),
		KCName:     firstNonEmpty(get("kcname"), get("kc_name")),
		User:       get("user"),
		Pass:       get("pass"),
		KSKS:       get("ksks"),
		KSJS:       get("ksjs"),
		StatusText: firstNonEmpty(get("status_text"), get("status")),
		Process:    get("process"),
		Remarks:    get("remarks"),
		Kcks:       get("kcks"),
		Kcjs:       get("kcjs"),
	}, nil
}
