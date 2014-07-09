package command

import (
	"encoding/json"
	"fmt"
	"net/http"
)

var (
	ErrUnknown = fmt.Errorf("Unknown error occurred")
)

// CheckResponse retrieves the error_description on error responses
func CheckResponse(resp *http.Response, expectedCode int) error {
	if resp.StatusCode == expectedCode {
		return nil
	}
	j := map[string]interface{}{}
	err := json.NewDecoder(resp.Body).Decode(&j)
	if err != nil {
		return ErrUnknown
	}
	errDesc, ok := j["error_description"].(string)
	if !ok {
		return ErrUnknown
	}
	return fmt.Errorf(errDesc)
}
