package cninfo

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
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

func TestCNINFOrequestWebapiNumericErrorResultcode(t *testing.T) {
	// CNINFO encodes resultcode as a JSON number, so the previous string-only
	// assertion read "" and treated this response as a success.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"resultcode":600,"resultmsg":"unauthorized"}`))
	}))
	defer server.Close()

	a := &CNINFOAdapter{client: http.DefaultClient}

	_, err := a.requestWebapi(context.Background(), nil, server.URL,
		[]webapiField{{name: "code"}},
		func(p map[string]interface{}) url.Values { return url.Values{} })
	if err == nil {
		t.Fatal("expected an error for numeric resultcode 600")
	}
	if !strings.Contains(err.Error(), "600") {
		t.Errorf("error %q does not carry the numeric code", err.Error())
	}
}

func TestCNINFOrequestWebapiNumericSuccessResultcode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"resultcode":200,"records":[{"symbol":"SZ000001"}],"totalCount":"1"}`))
	}))
	defer server.Close()

	a := &CNINFOAdapter{client: http.DefaultClient}

	_, err := a.requestWebapi(context.Background(), nil, server.URL,
		[]webapiField{{name: "code"}},
		func(p map[string]interface{}) url.Values { return url.Values{} })
	if err != nil {
		t.Fatalf("numeric success code 200 treated as an error: %v", err)
	}
}

func TestResultCodeString(t *testing.T) {
	tests := []struct {
		in   interface{}
		want string
	}{
		{"200", "200"},
		{"0", "0"},
		{float64(200), "200"},
		{float64(0), "0"},
		{float64(600), "600"},
		{nil, ""},
	}
	for _, tc := range tests {
		if got := resultCodeString(tc.in); got != tc.want {
			t.Errorf("resultCodeString(%v) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
