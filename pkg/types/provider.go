package types

// ProviderConfig 是模型供应商配置的公共类型。
// 该类型用于跨包创建模型实例。
type ProviderConfig struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	BaseURL string `json:"base_url"`
	APIKey  string `json:"api_key"`
	Model   string `json:"model"`
}

// IsLocal 判断供应商是否运行本地模型。
func (p ProviderConfig) IsLocal() bool {
	return p.ID == "ollama"
}
