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

func TestFieldSourceKeyMissingSourceIsNil(t *testing.T) {
	row := map[string]interface{}{
		"ORGNAME":  "贵州茅台酒股份有限公司",
		"F001V":    "Kweichow Moutai Co., Ltd.",
		"F007N":    125008.1601,
		"F018V":    "余思明",
		"MARKET":   "上交所",
		"ASECCODE": "600519",
	}

	if got := fieldSourceKey(row, "ORGNAME"); got != "贵州茅台酒股份有限公司" {
		t.Errorf("named source = %v, want company name", got)
	}

	// A positional source that the record does not carry must not be answered
	// with some other field's value. Before the fix this returned a value
	// scraped from a lexicographic sort of all values, so stock_profile rows
	// carried plausible-looking but entirely wrong data.
	for _, source := range []string{"0", "1", "2", "4", "25", "999"} {
		if got := fieldSourceKey(row, source); got != nil {
			t.Errorf("fieldSourceKey(%q) = %v, want nil", source, got)
		}
	}
}

func TestNormalizeWebapiRowStockProfile(t *testing.T) {
	row := map[string]interface{}{
		"ORGNAME":  "贵州茅台酒股份有限公司",
		"ASECCODE": "600519",
		"ASECNAME": "贵州茅台",
		"F001V":    "Kweichow Moutai Co., Ltd.",
		"F003V":    "陈华",
		"F018V":    "余思明",
		"F006D":    "2001-08-27",
		"F006V":    "564501",
		"F007N":    125008.1601,
		"F010D":    "1999-11-20",
		"F011V":    "www.moutaichina.com",
		"F032V":    "酒、饮料和精制茶制造业",
		"MARKET":   "上交所",
	}

	got := normalizeWebapiRow(row, WEBAPI_STOCK_PROFILE_FIELDS)

	want := map[string]interface{}{
		"company_name":         "贵州茅台酒股份有限公司",
		"a_share_code":         "600519",
		"a_share_name":         "贵州茅台",
		"english_name":         "Kweichow Moutai Co., Ltd.",
		"chairman":             "陈华",
		"legal_representative": "余思明",
		"listing_date":         "20010827",
		"founded_date":         "19991120",
		"postcode":             "564501",
		"website":              "www.moutaichina.com",
		"industry":             "酒、饮料和精制茶制造业",
		"market":               "上交所",
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %v, want %v", k, got[k], v)
		}
	}

	// Fields the record does not populate must stay empty, not borrow a value.
	for _, k := range []string{"b_share_code", "b_share_name", "h_share_code", "h_share_name", "former_short_name"} {
		if got[k] != "" {
			t.Errorf("%s = %v, want empty", k, got[k])
		}
	}
	if got["fax"] != "" {
		t.Errorf("fax = %v, want empty for an unpopulated source", got["fax"])
	}
}
