package test

type FakeCaptchaVerifier struct {
	Err error
}

func (v *FakeCaptchaVerifier) Verify(token string) error {
	return v.Err
}
