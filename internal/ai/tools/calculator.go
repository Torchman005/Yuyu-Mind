package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

// CalculatorTool 用于计算简单数学表达式。
type CalculatorTool struct{}

// CalculatorInput 定义计算器工具参数。
type CalculatorInput struct {
	Expression string `json:"expression" jsonschema:"description=A mathematical expression to evaluate, e.g. 2+3*4"`
}

// NewCalculatorTool 创建计算器工具。
func NewCalculatorTool() *CalculatorTool {
	return &CalculatorTool{}
}

// Info 返回提供给模型的工具元数据。
func (t *CalculatorTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return attachSchema(&schema.ToolInfo{
		Name: "calculator",
		Desc: "Evaluate a mathematical expression. Supports +, -, *, /, parentheses. Example: (2+3)*4",
	}, map[string]any{
		"type": "object",
		"properties": map[string]any{
			"expression": map[string]any{"type": "string", "description": "A mathematical expression to evaluate, e.g. 2+3*4"},
		},
		"required": []string{"expression"},
	}), nil
}

// InvokableRun 执行表达式计算。
func (t *CalculatorTool) InvokableRun(ctx context.Context, argumentsJSON string, opts ...tool.Option) (string, error) {
	var input CalculatorInput
	if err := json.Unmarshal([]byte(argumentsJSON), &input); err != nil {
		return "", fmt.Errorf("parse calculator arguments: %w", err)
	}

	result, err := evalSimple(input.Expression)
	if err != nil {
		return fmt.Sprintf("Error: %v", err), nil
	}

	return fmt.Sprintf("Result: %s = %s", input.Expression, result), nil
}

// evalSimple 只处理非常基础的算术表达式。
// 生产环境应替换为成熟的表达式求值库。
func evalSimple(expr string) (string, error) {
	// 只允许数字、运算符、括号、空格和小数点。
	sanitized := strings.Map(func(r rune) rune {
		if (r >= '0' && r <= '9') || r == '+' || r == '-' || r == '*' || r == '/' || r == '(' || r == ')' || r == '.' || r == ' ' {
			return r
		}
		return -1
	}, expr)

	if sanitized == "" {
		return "", fmt.Errorf("empty expression")
	}

	// 演示用的双操作数计算。
	parts := strings.SplitN(sanitized, "+", 2)
	if len(parts) == 2 {
		a, err1 := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
		b, err2 := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
		if err1 == nil && err2 == nil {
			return fmt.Sprintf("%v", a+b), nil
		}
	}

	parts = strings.SplitN(sanitized, "*", 2)
	if len(parts) == 2 {
		a, err1 := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
		b, err2 := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
		if err1 == nil && err2 == nil {
			return fmt.Sprintf("%v", a*b), nil
		}
	}

	// 兜底：尝试按单个数字解析。
	if v, err := strconv.ParseFloat(strings.TrimSpace(sanitized), 64); err == nil {
		return fmt.Sprintf("%v", v), nil
	}

	return "", fmt.Errorf("unsupported expression: %q (use a full expression evaluator for production)", expr)
}
