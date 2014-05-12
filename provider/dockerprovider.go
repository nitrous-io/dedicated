package provider

type DockerProvider struct{}

func (p *DockerProvider) Create(name string, publicKeys []string) (*ContainerInfo, error) {
	return nil, nil
}

func (p *DockerProvider) GetInfo(name string) (*ContainerInfo, error) {
	return nil, nil
}
