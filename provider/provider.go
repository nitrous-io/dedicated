package provider

type Provider interface {
	Create(name string, publicKeys []string, webhookUrl string) (info *ContainerInfo, err error)
	GetInfo(name string) (info *ContainerInfo, err error)
	//Terminate()
	//Start()
	//Stop()
	//Restart()
	//TakeSnapshots()
	//ForwardPort()
}

type ContainerInfo struct {
	Host string `json:host`
	Port uint16 `json:port`
}
