package provider

type Provider interface {
	List() (infoList []*ContainerInfo, err error)
	Create(name string, publicKeys []string) (info *ContainerInfo, err error)
	GetInfo(id string) (info *ContainerInfo, err error)
	//Terminate()
	//Start()
	//Stop()
	//Restart()
	//TakeSnapshots()
	//ForwardPort()
}

type ContainerInfo struct {
	Id   string `json:"id"`
	Name string `json:"name"`
	Host string `json:"host"`
	Port uint16 `json:"port"`
}
