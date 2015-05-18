package dns

type Provider interface {
	SetCNAME(src, dst string) error
	DeleteCNAME(domain string) error
}
