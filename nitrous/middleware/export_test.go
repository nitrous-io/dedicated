package middleware

func GetName(mw Middleware) string {
	return getName(mw)
}
