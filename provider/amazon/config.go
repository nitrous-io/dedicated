package amazon

type ProviderConfig struct {
	AccessKey        string `json:"access_key"`
	SecretKey        string `json:"secret_key"`
	DefaultPublicKey string `json:"public_key"`
}

type HostConfig struct {
	SourceAmi        string   `json:"source_ami"`
	AvailabilityZone string   `json:"availability_zone"`
	InstanceType     string   `json:"instance_type"`
	RawRegion        string   `json:"region"`
	PublicKeys       []string `json:"public_keys"`
}
