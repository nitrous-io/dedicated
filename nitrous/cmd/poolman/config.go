package main

import (
	"encoding/json"
	"fmt"
	"io"
)

type Config struct {
	HostbuilderURL string         `json:"hostbuilder_url"`
	AccessKey      string         `json:"access_key"`
	SecretKey      string         `json:"secret_key"`
	Env            string         `json:"env"`
	Regions        []regionConfig `json:"regions"`
}

type regionConfig struct {
	ID              string       `json:"id"`
	SubnetID        string       `json:"subnet_id"`
	SecurityGroupID string       `json:"security_group_id"`
	InstancePool    []poolConfig `json:"instance_pool"`
}

type poolConfig struct {
	Type string `json:"type"`
	Min  int    `json:"min"`
}

func NewConfig(r io.Reader) (*Config, error) {
	c := &Config{}
	if err := json.NewDecoder(r).Decode(c); err != nil {
		return nil, fmt.Errorf("error parsing config: %v", err)
	}
	return c, nil
}

func (c *Config) String() string {
	type c2 Config
	cs := c2(*c)
	cs.AccessKey = "(REDACTED)"
	cs.SecretKey = "(REDACTED)"
	return fmt.Sprintf("%+v", cs)
}
