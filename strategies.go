// Package reqstrategy provides functions for coordinating http.Client calls.
// It wraps typical call strategies like making simultaneous requests, retrying, racing etc.
// Package is not aiming to replace standard library's client and Request types only provides
// the way to invoke them in different manners
//
// First, define what response counts successful, there are two helpers adding rules to request context
//
//   req, _ = http.NewRequest("GET", "http://localhost/", nil)
//   req = WithStatusRequired(req, 200, 404)
//
// or do adavanced validation using custom logic
//
//   req, _ = http.NewRequest("GET", "http://localhost/", nil)
//   req = WithValidator(req, func(resp *http.Response) error {
//     if resp.StatusCode != http.StatusOK {
//       return fmt.Error("oops")
//     }
//     return nil
//   })
//
// then use one of the Do, Race, All, Retry to invoke the request with following response validation
//
package reqstrategy

import (
	"fmt"
	"net/http"
	"time"
)

// Do is not much different from calling client.Do(request) except it runs the
// response validation. See WithValidator and WithSTatusRequired
func Do(client *http.Client, request *http.Request) (*http.Response, error) {
	resp, err := client.Do(request)
	if err != nil {
		return resp, err
	}
	validators, _ := request.Context().Value(keyValidators).([]validator)
	for _, validate := range validators {
		if err := validate(resp); err != nil {
			return resp, err
		}
	}
	return resp, nil
}

// Race runs requests simultaneously returning first successulf result or error if all failed.
// Once result is determined all requests are cancelled through the context.
func Race(client *http.Client, requests ...*http.Request) (*http.Response, error) {
	results := make(chan result, len(requests))
	stop := make(chan struct{})
	defer close(stop)

	for i, r := range requests {
		go do(client, r, i, stop, results)
	}

	var received int
	for res := range results {
		if res.err == nil {
			return res.response, nil
		}
		received++
		if received == len(requests) {
			break
		}
	}

	return nil, fmt.Errorf("all requests failed")
}

// All runs requests simultaneously returning responses in same order or error if at least one request failed.
// Once result is determined all requests are cancelled through the context.
func All(client *http.Client, requests ...*http.Request) ([]*http.Response, error) {
	results := make(chan result, len(requests))
	stop := make(chan struct{})
	defer close(stop)

	for i, r := range requests {
		go do(client, r, i, stop, results)
	}

	var received int
	responses := make([]*http.Response, len(requests), len(requests))
	for res := range results {
		if res.err != nil {
			return nil, res.err
		}
		received++
		responses[res.order] = res.response
		if received == len(requests) {
			break
		}
	}

	return responses, nil
}

// Some runs requests simultaneously returning responses for successful requests and <nil> for failed ones.
// Error is returned only if all requests failed.
func Some(client *http.Client, requests ...*http.Request) ([]*http.Response, error) {
	results := make(chan result, len(requests))
	stop := make(chan struct{})
	defer close(stop)

	for i, r := range requests {
		go do(client, r, i, stop, results)
	}

	var received, successful int
	responses := make([]*http.Response, len(requests), len(requests))
	for res := range results {
		received++
		if res.err == nil {
			successful++
			responses[res.order] = res.response
		}
		if received == len(requests) {
			break
		}
	}
	if successful == 0 {
		return nil, fmt.Errorf("all requests failed")
	}

	return responses, nil
}

// Retry re-attempts request with provided intervals. By manually providing intervals sequence you
// can have different wait strategies like exponential back-off (time.Second, 2 * time.Second, 4 * time.Second)
// or just multiple reties after same interval (time.Second, time.Second, time.Second). If Request had a context
// with timeout cancelation then it will be applied to entire chain
func Retry(client *http.Client, request *http.Request, intervals ...time.Duration) (*http.Response, error) {
	ctx := request.Context()
	for true {
		response, err := Do(client, request)
		if err == nil {
			return response, nil
		}
		if len(intervals) == 0 {
			return response, err
		}
		select {
		case <-time.After(intervals[0]):
			intervals = intervals[1:]
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return nil, fmt.Errorf("retry loop failed")
}
