package service

import (
	"math"
	"strconv"
	"strings"
)

// 本文件提供各渠道 usage 解析共用的宽松 JSON 取值 helper：
// 上游响应字段可能缺失或类型变化（数字可能是 JSON number、整数或
// 数字字符串），缺失/类型不兼容一律兜底为零值，聚合了此前分散在各
// usage 文件中的同名实现（仅前缀不同、容错逻辑近似）。

// jsonMapGet 按 keys 顺序返回第一个存在且为对象的字段。
// 未命中返回 nil, false。
func jsonMapGet(m map[string]any, keys ...string) (map[string]any, bool) {
	for _, key := range keys {
		value, ok := m[key]
		if !ok {
			continue
		}
		if nested, ok := value.(map[string]any); ok {
			return nested, true
		}
	}
	return nil, false
}

// jsonMapSlice 按 keys 顺序返回第一个存在且为数组的字段，
// 仅保留其中的对象元素；未命中返回 nil。
func jsonMapSlice(m map[string]any, keys ...string) []map[string]any {
	for _, key := range keys {
		raw, ok := m[key]
		if !ok {
			continue
		}
		items, ok := raw.([]any)
		if !ok {
			continue
		}
		result := make([]map[string]any, 0, len(items))
		for _, item := range items {
			if nested, ok := item.(map[string]any); ok {
				result = append(result, nested)
			}
		}
		return result
	}
	return nil
}

// jsonString 按 keys 顺序返回第一个存在且为字符串的字段（去除首尾空白）。
func jsonString(m map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := m[key]; ok {
			if s, ok := value.(string); ok {
				return strings.TrimSpace(s)
			}
		}
	}
	return ""
}

// jsonFloatOK 按 keys 顺序返回第一个存在且可解析为数字的字段
// （JSON number 或数字字符串）；NaN 视为缺失，返回 ok=false。
func jsonFloatOK(m map[string]any, keys ...string) (float64, bool) {
	for _, key := range keys {
		value, ok := m[key]
		if !ok {
			continue
		}
		switch n := value.(type) {
		case float64:
			if math.IsNaN(n) {
				continue
			}
			return n, true
		case string:
			if f, err := strconv.ParseFloat(strings.TrimSpace(n), 64); err == nil && !math.IsNaN(f) {
				return f, true
			}
		}
	}
	return 0, false
}

// jsonFloat 是 jsonFloatOK 的免 ok 版本，缺失/无效时返回 0。
func jsonFloat(m map[string]any, keys ...string) float64 {
	f, _ := jsonFloatOK(m, keys...)
	return f
}

// jsonInt64Clamped 按 keys 顺序返回第一个存在且可解析为整数的字段
// （JSON number 或数字字符串）。对 float64 值统一做 int64 范围 clamp，
// 避免上游返回超大数字（如 1e30）时 int64(n) 溢出产生不可预期的展示值。
func jsonInt64Clamped(m map[string]any, keys ...string) int64 {
	for _, key := range keys {
		value, ok := m[key]
		if !ok {
			continue
		}
		switch n := value.(type) {
		case float64:
			if math.IsNaN(n) {
				continue
			}
			return clampToInt64(n)
		case string:
			s := strings.TrimSpace(n)
			if i, err := strconv.ParseInt(s, 10, 64); err == nil {
				return i
			}
			if f, err := strconv.ParseFloat(s, 64); err == nil && !math.IsNaN(f) {
				return clampToInt64(f)
			}
		}
	}
	return 0
}

// clampToInt64 将 float64 安全截断为 int64：超出 int64 可表示范围时
// clamp 到 math.MaxInt64 / math.MinInt64（float64(math.MaxInt64) 即 2^63，
// 大于等于它的值全部 clamp，避免负数乱码式展示值）。
func clampToInt64(f float64) int64 {
	if f >= float64(math.MaxInt64) {
		return math.MaxInt64
	}
	if f <= float64(math.MinInt64) {
		return math.MinInt64
	}
	return int64(f)
}

// jsonInt 是 jsonInt64Clamped 的 int 版本，用于需要 int 的展示字段。
func jsonInt(m map[string]any, keys ...string) int {
	return int(jsonInt64Clamped(m, keys...))
}

// jsonBool 按 keys 顺序返回第一个存在且为布尔值的字段。
func jsonBool(m map[string]any, keys ...string) bool {
	for _, key := range keys {
		if value, ok := m[key]; ok {
			if b, ok := value.(bool); ok {
				return b
			}
		}
	}
	return false
}
