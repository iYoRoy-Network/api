package config

// ServerConfig 服务器模块配置
type ServerConfig struct {
	ServerAddress string `koanf:"server_address"`
}

// DefaultServerConfig 返回服务器模块的默认配置
func DefaultServerConfig() ServerConfig {
	return ServerConfig{
		ServerAddress: ":8080",
	}
}
