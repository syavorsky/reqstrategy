package reqstrategy

import (
	"context"
	"fmt"
	"net/http"
)

// WithValidator introduces a response validator function to the context to be used in Do/Race/All/Some/Retry
func WithValidator(r *http.Request, validate validator) *http.Request {
	ctx := r.Context()

	validators, _ := ctx.Value(keyValidators).([]validator)
	validators = append(validators, validate)

	ctx = context.WithValue(ctx, keyValidators, validators)
	return r.WithContext(ctx)
}

// WithStatusRequired adds the response validator by listing acceptable status codes
func WithStatusRequired(r *http.Request, codes ...int) *http.Request {
	return WithValidator(r, func(r *http.Response) error {
		for _, code := range codes {
			if r.StatusCode == code {
				return nil
			}
		}
		return fmt.Errorf("%s %s: expected response status %v, got %d", r.Request.Method, r.Request.URL, codes, r.StatusCode)
	})
}
