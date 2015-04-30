package redis

type Config struct {
	Host string `json:"host"`
	Port string `json:"port"`
	DB   int64  `json:"database"`
}
