package provider

type DockerProvider struct{}

func (p *DockerProvider) List() ([]*ContainerInfo, error) {
	return nil, nil
}

func (p *DockerProvider) Create(name string, image string, publicKeys []string) (*ContainerInfo, error) {
	return nil, nil
}

func (p *DockerProvider) GetInfo(name string) (*ContainerInfo, error) {
	return nil, nil
}
