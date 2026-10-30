package client

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/slop-place/terraform-provider-opwan/internal/opwanapi"
)

const (
	CODE_NOT_FOUND          = opwanapi.ProblemCode("not_found")
	CODE_LOCATION_LOCKED    = opwanapi.ProblemCode("location_locked")
	CODE_UNAUTHENTICATED    = opwanapi.ProblemCode("unauthenticated")
	CODE_INSUFFICIENT_SCOPE = opwanapi.ProblemCode("insufficient_scope")
)

// APIError is a non-2xx answer from opwan, carrying its problem document when there was one.
type APIError struct {
	StatusCode int
	Problem    *opwanapi.Problem
	Body       string
}

func (e *APIError) Error() string {
	if e.Problem == nil {
		return fmt.Sprintf("opwan API returned HTTP %d: %s", e.StatusCode, strings.TrimSpace(e.Body))
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%s (HTTP %d, %s)", e.Problem.Detail, e.StatusCode, e.Problem.Code)
	if e.Problem.Errors != nil {
		for _, fieldErr := range *e.Problem.Errors {
			fmt.Fprintf(&b, "\n  - %s: %s", fieldErr.Field, fieldErr.Message)
		}
	}
	if e.Problem.RequestId != nil {
		fmt.Fprintf(&b, "\nrequest id: %s", *e.Problem.RequestId)
	}
	if e.Problem.Code == CODE_UNAUTHENTICATED {
		b.WriteString("\nSet the provider's api_token, or OPWAN_API_TOKEN, to a valid opwan API token.")
	}
	return b.String()
}

func HasCode(err error, code opwanapi.ProblemCode) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) && apiErr.Problem != nil && apiErr.Problem.Code == code
}

func IsNotFound(err error) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound
}

func checkResponse(resp *http.Response, body []byte) error {
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}

	apiErr := &APIError{StatusCode: resp.StatusCode, Body: string(body)}
	var problem opwanapi.Problem
	if json.Unmarshal(body, &problem) == nil && problem.Code != "" {
		apiErr.Problem = &problem
	}
	return apiErr
}
