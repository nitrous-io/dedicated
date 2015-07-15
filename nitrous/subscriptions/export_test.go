package subscriptions

func SetCouponGetFn(fn couponGetFunc) {
	couponGetFn = fn
}

var DefaultCouponGetFn couponGetFunc

func init() {
	DefaultCouponGetFn = couponGetFn
}
