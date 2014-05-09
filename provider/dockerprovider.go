package provider

type DockerProvider struct{}

func (p *DockerProvider) Create(name string, publicKeys []string, webhookUrl string) (string, uint16, error) {
	return "", 0, nil
}
