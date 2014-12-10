package biller

func (b *Biller) CreateOrUpdateInvoice() (*InvoiceResult, error) {
	return b.createOrUpdateInvoice()
}

func (b *Biller) ProcessStripe(i *InvoiceResult) error {
	return b.processStripe(i)
}
