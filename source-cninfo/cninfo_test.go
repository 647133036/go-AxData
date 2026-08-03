package cninfo

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestCNINFOrequestWebapi_429(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(429)
		w.Write([]byte(`{"error":"Too Many Requests"}`))
	}))
	defer server.Close()

	a := &CNINFOAdapter{client: http.DefaultClient}

	params := map[string]interface{}{
		"pageNum":  1,
		"pageSize": 10,
	}

	_, err := a.requestWebapi(context.Background(), params, server.URL,
		[]webapiField{{name: "code"}},
		func(p map[string]interface{}) url.Values { return url.Values{} })
	if err == nil {
		t.Fatal("Expected 429 error")
	}
}

func TestCNINFOrequestWebapiSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"records":[{"symbol":"SZ000001"}],"totalCount":"1"}`))
	}))
	defer server.Close()

	a := &CNINFOAdapter{client: http.DefaultClient}

	_, err := a.requestWebapi(context.Background(), nil, server.URL,
		[]webapiField{{name: "code"}},
		func(p map[string]interface{}) url.Values { return url.Values{} })
	if err != nil {
		t.Fatalf("requestWebapi failed: %v", err)
	}
}

func TestCNINFOrequestWebapiNoRecords(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"result":{"records":[],"totalCount":"0"},"status":"0"}`))
	}))
	defer server.Close()

	a := &CNINFOAdapter{client: http.DefaultClient}

	_, err := a.requestWebapi(context.Background(), nil, server.URL,
		[]webapiField{{name: "code"}},
		func(p map[string]interface{}) url.Values { return url.Values{} })
	if err == nil {
		t.Fatal("Expected 'no records' error")
	}
}

func TestCNINFOrequestWebapiInvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`not json`))
	}))
	defer server.Close()

	a := &CNINFOAdapter{client: http.DefaultClient}

	_, err := a.requestWebapi(context.Background(), nil, server.URL,
		[]webapiField{{name: "code"}},
		func(p map[string]interface{}) url.Values { return url.Values{} })
	if err == nil {
		t.Fatal("Expected JSON error")
	}
}
