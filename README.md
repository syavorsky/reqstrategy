# reqstrategy

Package `reqstrategy` provides functions for coordinating `http.Client` calls. It wraps typical call strategies like making simultaneous requests, retrying, racing etc.

First, define what request counts successful, there are two helpers adding rules to the request context

```go
req, _ = http.NewRequest("GET", "http://localhost/", nil)
req = WithStatusRequired(req, 200, 404)
```

or do adavanced validation using custom logic

```go
req, _ = http.NewRequest("GET", "http://localhost/", nil)
req = WithValidator(req, func(resp *http.Response) error {
  if resp.StatusCode != http.StatusOK {
    return fmt.Error("oops")
  }
  return nil
})
```

Then, make a call

`Do()` is not much different from calling `client.Do(request)` except it runs the response validation. See WithValidator and WithSTatusRequired

```go
resp, err := Do(http.DefaultClient, req)
```

`Retry()` re-attempts request with provided intervals. By manually providing intervals sequence you can have different wait strategies like exponential back-off (`time.Second, 2 * time.Second, 4 * time.Second`) or just multiple reties after same interval (`time.Second, time.Second, time.Second`). If Request had a context with timeout cancelation then it will be applied to entire chain

```go
resp, err := Retry(http.DefaultClient, req, time.Second, time.Second, time.Second)
```

`Race()` runs multiple requests simultaneously returning first successulf result or error if all failed. Once result is determined all requests are cancelled through the context.

```go
resps, err := Race(http.DefaultClient, req0, req1, reqX)
```

`All()` runs requests simultaneously returning responses in same order or error if at least one request failed. Once result is determined all requests are cancelled through the context.

```go
resps, err := All(http.DefaultClient, req0, req1, reqX)
```

`Some()` runs requests simultaneously returning responses for successful requests and `<nil>` for failed ones. Error is returned only if all requests failed.

```go
resps, err := Some(http.DefaultClient, req0, req1, reqX)
```