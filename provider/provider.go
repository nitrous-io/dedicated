package provider

type Provider interface {
	Create() error
}

func GetProvider(name string) Provider {
}
