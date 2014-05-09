package freightertest

import (
	"fmt"
	"net/http"
	"net/url"
	"reflect"
	"strings"

	"github.com/motain/gocheck"
)

func NewRequest(method string, urlStr string, params url.Values) (*http.Request, error) {
	u, err := url.Parse(urlStr)
	if err != nil {
		return nil, err
	}
	if params != nil {
		u.RawQuery = params.Encode()
	}
	return http.NewRequest(method, u.String(), nil)
}

func NewFormRequest(method string, urlStr string, params url.Values) (*http.Request, error) {
	req, err := http.NewRequest(method, urlStr, strings.NewReader(params.Encode()))
	if err == nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	return req, err
}

type containsChecker struct {
	*gocheck.CheckerInfo
}

var Contains gocheck.Checker = &containsChecker{
	&gocheck.CheckerInfo{Name: "Contains", Params: []string{"expected", "to contain"}},
}

func (checker *containsChecker) Check(params []interface{}, names []string) (result bool, error string) {
	defer func() {
		if v := recover(); v != nil {
			result = false
			error = fmt.Sprint(v)
		}
	}()
	exists := false
	for _, v := range params[0].([]interface{}) {
		if reflect.DeepEqual(v, params[1]) {
			exists = true
			break
		}
	}
	return exists, ""
}
