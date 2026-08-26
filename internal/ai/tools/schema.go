package tools

import (
	"encoding/json"
	"fmt"

	"github.com/cloudwego/eino/schema"
	jsonschema "github.com/eino-contrib/jsonschema"
)

// jsonschemaParamsFromMap 把一个 JSON Schema 对象(map[string]any)转成 Eino 的工具参数 schema(ParamsOneOf)。
// 让 Worker/Planner 模型能正确构造工具参数（否则只能靠猜字段名，导致传错如 file_path vs path）。
func jsonschemaParamsFromMap(m map[string]any) (*schema.ParamsOneOf, error) {
	if m == nil {
		return nil, nil
	}
	data, err := json.Marshal(m)
	if err != nil {
		return nil, fmt.Errorf("marshal tool schema: %w", err)
	}
	var s jsonschema.Schema
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parse tool schema: %w", err)
	}
	return schema.NewParamsOneOfByJSONSchema(&s), nil
}

// BuildToolParams 把 JSON Schema map 转成 Eino 工具参数结构，供其它包（如目录插件）复用。
func BuildToolParams(m map[string]any) (*schema.ParamsOneOf, error) {
	return jsonschemaParamsFromMap(m)
}

// attachSchema 把 JSON Schema 挂到 ToolInfo 上（失败时静默保留原 Info）。
func attachSchema(info *schema.ToolInfo, m map[string]any) *schema.ToolInfo {
	if p, err := jsonschemaParamsFromMap(m); err == nil && p != nil {
		info.ParamsOneOf = p
	}
	return info
}
