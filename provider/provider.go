package provider

type Provider interface {
	Create(name string, publicKeys []string, webhookUrl string) (host string, port uint16, err error)
	//GetInfo()
	//Terminate()
	//Start()
	//Stop()
	//Restart()
	//TakeSnapshots()
	//ForwardPort()
}
