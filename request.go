package reqstrategy

import (
	"context"
	"net/http"
)

type key string

const keyValidators key = "validators"

type validator = func(r *http.Response) error

type result struct {
	order    int
	response *http.Response
	err      error
}

func do(client *http.Client, r *http.Request, order int, stop <-chan struct{}, results chan<- result) {
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	go func() {
		<-stop
		cancel()
	}()
	response, err := Do(client, r.WithContext(ctx))
	results <- result{order, response, err}
}
