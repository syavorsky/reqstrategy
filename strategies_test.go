package reqstrategy

import (
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type transport func(*http.Request) (*http.Response, error)

func (t transport) RoundTrip(r *http.Request) (*http.Response, error) {
	return t(r)
}

func newRequest(t *testing.T, path ...string) *http.Request {
	request, err := http.NewRequest("GET", "http://localhost/"+strings.Join(path, "/"), nil)
	if err != nil {
		t.Fatalf(`failed to create "%s" request: %s`, path, err)
	}
	return request
}

func newClient(roundTrip func(r *http.Request) (*http.Response, error)) *http.Client {
	return &http.Client{Transport: transport(roundTrip)}
}

func Test_Do(t *testing.T) {
	client := newClient(func(r *http.Request) (*http.Response, error) {
		return &http.Response{Request: r, StatusCode: 200}, nil
	})

	response, err := Do(client, newRequest(t))
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	if response.StatusCode != 200 {
		t.Fatalf("unexpected response")
	}
}

func Test_Do_error(t *testing.T) {
	client := newClient(func(r *http.Request) (*http.Response, error) {
		return nil, fmt.Errorf("request failed")
	})

	response, err := Do(client, newRequest(t))
	if response != nil {
		t.Fatalf(`response should be <nil>, got %v`, response)
	}
	if err == nil {
		t.Fatal("expected error, got <nil>")
	}
	if err.Error() != "Get http://localhost/: request failed" {
		t.Fatalf(`expected error "Get http://localhost/: request failed", got "%s"`, err.Error())
	}
}

func Test_Do_validation(t *testing.T) {
	client := newClient(func(r *http.Request) (*http.Response, error) {
		return &http.Response{Request: r, StatusCode: 404}, nil
	})

	req := WithStatusRequired(newRequest(t), 200)
	resp, err := Do(client, req)

	if resp == nil || resp.StatusCode != 404 {
		t.Fatal("response expected")
	}
	if err == nil {
		t.Fatal("validator is expected to fail")
	}
	want := "GET http://localhost/: expected response status [200], got 404"
	if err.Error() != want {
		t.Fatalf(`expected "%s" error, got "%s"`, want, err.Error())
	}
}

func Test_WithStatusRequired(t *testing.T) {
	req := WithStatusRequired(newRequest(t), 200)

	validators, _ := req.Context().Value(keyValidators).([]validator)
	if len(validators) != 1 {
		t.Fatalf("expected 1 validator to be set, got %d", len(validators))
	}

	res := &http.Response{Request: req, StatusCode: 404}
	err := validators[0](res)
	if err == nil {
		t.Fatal("validator is expected to fail")
	}

	want := "GET http://localhost/: expected response status [200], got 404"
	if err.Error() != want {
		t.Fatalf(`expected "%s" error, got "%s"`, want, err.Error())
	}
}

func Test_Race(t *testing.T) {
	client := newClient(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/a":
			<-time.After(200 * time.Millisecond)
			return &http.Response{Request: r, StatusCode: 200}, nil
		case "/b":
			<-time.After(100 * time.Millisecond)
			return &http.Response{Request: r, StatusCode: 500}, nil
		case "/c":
			<-time.After(300 * time.Millisecond)
			return &http.Response{Request: r, StatusCode: 200}, nil
		default:
			panic("wrong URL: " + r.URL.String())
		}
	})

	response, err := Race(client,
		WithStatusRequired(newRequest(t, "a"), 200), // second fastest
		WithStatusRequired(newRequest(t, "b"), 200), // fastest but failed
		WithStatusRequired(newRequest(t, "c"), 200),
	)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	if response.Request.URL.Path != "/a" {
		t.Fatalf(`expected "/a" to win, got "%s"`, response.Request.URL)
	}
}

func Test_Race_all_failed(t *testing.T) {
	client := newClient(func(r *http.Request) (*http.Response, error) {
		return &http.Response{Request: r, StatusCode: 500}, nil
	})

	response, err := Race(client,
		WithStatusRequired(newRequest(t, "a"), 200),
		WithStatusRequired(newRequest(t, "b"), 200),
		WithStatusRequired(newRequest(t, "c"), 200),
	)
	if response != nil {
		t.Fatalf("expected <nil> response")
	}
	if err == nil {
		t.Fatalf("expected error")
	}
	if err.Error() != "all requests failed" {
		t.Fatalf(`expected "all requests failed" error, got "%s"`, err.Error())
	}
}

func Test_All(t *testing.T) {
	client := newClient(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/a":
			<-time.After(200 * time.Millisecond)
			return &http.Response{Request: r, StatusCode: 200}, nil
		case "/b":
			<-time.After(100 * time.Millisecond)
			return &http.Response{Request: r, StatusCode: 200}, nil
		case "/c":
			<-time.After(300 * time.Millisecond)
			return &http.Response{Request: r, StatusCode: 200}, nil
		default:
			panic("wrong URL: " + r.URL.String())
		}
	})

	responses, err := All(client,
		WithStatusRequired(newRequest(t, "a"), 200), // second fastest
		WithStatusRequired(newRequest(t, "b"), 200), // fastest but failed
		WithStatusRequired(newRequest(t, "c"), 200),
	)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	for i, path := range []string{"/a", "/b", "/c"} {
		if responses[i].Request.URL.Path != path {
			t.Fatalf(`expected #%d response to be from "%s", got "%s"`, i, path, responses[i].Request.URL.Path)
		}
	}
}

func Test_All_error(t *testing.T) {
	client := newClient(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/a":
			<-time.After(200 * time.Millisecond)
			return &http.Response{Request: r, StatusCode: 200}, nil
		case "/b":
			<-time.After(100 * time.Millisecond)
			return &http.Response{Request: r, StatusCode: 500}, nil
		case "/c":
			<-time.After(300 * time.Millisecond)
			return &http.Response{Request: r, StatusCode: 200}, nil
		default:
			panic("wrong URL: " + r.URL.String())
		}
	})

	responses, err := All(client,
		WithStatusRequired(newRequest(t, "a"), 200), // second fastest
		WithStatusRequired(newRequest(t, "b"), 200), // fastest but failed
		WithStatusRequired(newRequest(t, "c"), 200),
	)
	if responses != nil {
		t.Fatalf("expected no responses, got %v", responses)
	}
	if err == nil {
		t.Fatal("expected error")
	}
	want := "GET http://localhost/b: expected response status [200], got 500"
	if err.Error() != want {
		t.Fatalf(`expected "%s" error, got "%s"`, want, err.Error())
	}
}

func Test_Some(t *testing.T) {
	client := newClient(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/a":
			<-time.After(200 * time.Millisecond)
			return &http.Response{Request: r, StatusCode: 200}, nil
		case "/b":
			<-time.After(100 * time.Millisecond)
			return &http.Response{Request: r, StatusCode: 500}, nil
		case "/c":
			<-time.After(300 * time.Millisecond)
			return &http.Response{Request: r, StatusCode: 200}, nil
		default:
			panic("wrong URL: " + r.URL.String())
		}
	})

	responses, err := Some(client,
		WithStatusRequired(newRequest(t, "a"), 200), // second fastest
		WithStatusRequired(newRequest(t, "b"), 200), // fastest but failed
		WithStatusRequired(newRequest(t, "c"), 200),
	)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	if responses[0].Request.URL.Path != "/a" {
		t.Fatalf(`expected #0 response to be from "/a", got "%s"`, responses[0].Request.URL.Path)
	}
	if responses[1] != nil {
		t.Fatalf(`expected #1 response to be <nil>, got %v`, responses[1])
	}
	if responses[2].Request.URL.Path != "/c" {
		t.Fatalf(`expected #2 response to be from "/c", got "%s"`, responses[2].Request.URL.Path)
	}
}

func Test_Some_all_failed(t *testing.T) {
	client := newClient(func(r *http.Request) (*http.Response, error) {
		return &http.Response{Request: r, StatusCode: 500}, nil
	})

	response, err := Some(client,
		WithStatusRequired(newRequest(t, "a"), 200),
		WithStatusRequired(newRequest(t, "b"), 200),
		WithStatusRequired(newRequest(t, "c"), 200),
	)
	if response != nil {
		t.Fatalf("expected <nil> response")
	}
	if err == nil {
		t.Fatalf("expected error")
	}
	if err.Error() != "all requests failed" {
		t.Fatalf(`expected "all requests failed" error, got "%s"`, err.Error())
	}
}

func Test_Retry(t *testing.T) {
	var count int32
	client := newClient(func(r *http.Request) (*http.Response, error) {
		if atomic.AddInt32(&count, 1) <= 2 {
			return &http.Response{Request: r, StatusCode: 500}, nil
		}
		return &http.Response{Request: r, StatusCode: 200}, nil
	})

	resp, err := Retry(
		client,
		WithStatusRequired(newRequest(t), 200),
		100*time.Millisecond,
		100*time.Millisecond,
		100*time.Millisecond,
	)

	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf(`expected response status 200, got %d`, resp.StatusCode)
	}
	if count != 3 {
		t.Fatalf(`expected 3 calls to be made, got %d`, count)
	}
}

func Test_Retry_error(t *testing.T) {
	client := newClient(func(r *http.Request) (*http.Response, error) {
		return &http.Response{Request: r, StatusCode: 500}, nil
	})

	resp, err := Retry(
		client,
		WithStatusRequired(newRequest(t), 200),
		100*time.Millisecond,
		100*time.Millisecond,
		100*time.Millisecond,
	)

	if resp == nil {
		t.Fatalf("response expected")
	}
	if err == nil {
		t.Fatal("expected error")
	}
	want := "GET http://localhost/: expected response status [200], got 500"
	if err.Error() != want {
		t.Fatalf(`expected "%s" error, got "%s"`, want, err.Error())
	}
}
