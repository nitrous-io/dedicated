package hostbuilder

import (
	"encoding/json"
	"fmt"
	"io"
)

type Config struct {
	Address   string                    `json:"address"`
	Providers map[string]providerConfig `json:"providers"`

	// librato settings
	LibratoUser  string `json:"librato_user"`
	LibratoToken string `json:"librato_token"`

	// max time to wait for graceful stop, in seconds
	StopTimeout int `json:"stop_timeout"`

	// start the server in debug mode
	Debug bool `json:"debug"`

	LogLevel string `json:"log_level"`

	QueueType    string `json:"queue_type"`
	RabbitMQAddr string `json:"rabbitmq_addr"`

	// redis setting
	RedisConfig redisConfig `json:"redis"`

	// whether partial provider List() calls are considered an error
	AllowPartialList bool `json:"allow_partial_list"`

	// CPU profile dump folder
	ProfileDumpFolder string `json:"profile_dump_folder"`
}

type providerConfig map[string]string

type redisConfig struct {
	Host string `json:"host"`
	Port string `json:"port"`
	DB   int64  `json:"database"`
}

func NewConfig(r io.Reader) (*Config, error) {
	c := &Config{}
	if err := json.NewDecoder(r).Decode(c); err != nil {
		return nil, fmt.Errorf("error decoding config, err: %v", err)
	}
	return c, nil
}
