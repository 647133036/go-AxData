package cninfo

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

var (
	CNINFO_STOCK_INDEX_URL      = "https://www.cninfo.com.cn/new/data/szse_stock.json"
	CNINFO_ANNOUNCEMENT_QUERY_URL = "https://www.cninfo.com.cn/new/hisAnnouncement/query"
	CNINFO_STATIC_BASE          = "https://static.cninfo.com.cn/"
	CNINFO_IRM_KEYWORD_URL      = "https://irm.cninfo.com.cn/newircs/index/queryKeyboardInfo"
	CNINFO_IRM_QUESTION_URL     = "https://irm.cninfo.com.cn/newircs/company/question"
	CNINFO_IRM_DETAIL_URL       = "https://irm.cninfo.com.cn/newircs/question/getQuestionDetail"

	WEBAPI_STOCK_PROFILE_URL       = "https://webapi.cninfo.com.cn/api/sysapi/p_sysapi1133"
	WEBAPI_STOCK_ALLOTMENT_URL     = "https://webapi.cninfo.com.cn/api/stock/p_stock2232"
	WEBAPI_STOCK_DIVIDEND_URL      = "https://webapi.cninfo.com.cn/api/sysapi/p_sysapi1139"
	WEBAPI_STOCK_HOLD_CHANGE_URL   = "https://webapi.cninfo.com.cn/api/sysapi/p_sysapi1029"
	WEBAPI_STOCK_HOLD_CONTROL_URL  = "https://webapi.cninfo.com.cn/api/sysapi/p_sysapi1033"
	WEBAPI_STOCK_HOLD_NUM_URL      = "https://webapi.cninfo.com.cn/api/sysapi/p_sysapi1034"
	WEBAPI_STOCK_INDUSTRY_CATEGORY_URL = "https://webapi.cninfo.com.cn/api/stock/p_public0002"
	WEBAPI_STOCK_INDUSTRY_PE_RATIO_URL = "http://webapi.cninfo.com.cn/api/sysapi/p_sysapi1087"
	WEBAPI_STOCK_RANK_FORECAST_URL = "http://webapi.cninfo.com.cn/api/sysapi/p_sysapi1089"
	WEBAPI_STOCK_IPO_SUMMARY_URL   = "https://webapi.cninfo.com.cn/api/sysapi/p_sysapi1134"
	WEBAPI_STOCK_NEW_GH_URL        = "https://webapi.cninfo.com.cn/api/sysapi/p_sysapi1098"
	WEBAPI_STOCK_NEW_IPO_URL       = "https://webapi.cninfo.com.cn/api/sysapi/p_sysapi1097"
	WEBAPI_STOCK_SHARE_CHANGE_URL  = "https://webapi.cninfo.com.cn/api/stock/p_stock2215"
	WEBAPI_STOCK_INDUSTRY_CHANGE_URL = "https://webapi.cninfo.com.cn/api/stock/p_stock2110"
	WEBAPI_FUND_ASSET_ALLOCATION_URL = "https://webapi.cninfo.com.cn/api/sysapi/p_sysapi1114"
	WEBAPI_FUND_INDUSTRY_ALLOCATION_URL = "https://webapi.cninfo.com.cn/api/sysapi/p_sysapi1113"
	WEBAPI_FUND_STOCK_URL          = "https://webapi.cninfo.com.cn/api/sysapi/p_sysapi1112"
	WEBAPI_STOCK_CG_EQUITY_MORTGAGE_URL = "https://webapi.cninfo.com.cn/api/sysapi/p_sysapi1094"
	WEBAPI_STOCK_CG_GUARANTEE_URL  = "https://webapi.cninfo.com.cn/api/sysapi/p_sysapi1054"
	WEBAPI_STOCK_CG_LAWSUIT_URL    = "https://webapi.cninfo.com.cn/api/sysapi/p_sysapi1055"
	WEBAPI_STOCK_HOLD_MANAGEMENT_DETAIL_URL = "https://webapi.cninfo.com.cn/api/sysapi/p_sysapi1030"
	WEBAPI_BOND_CORPORATE_ISSUE_URL = "https://webapi.cninfo.com.cn/api/sysapi/p_sysapi1122"
	WEBAPI_BOND_COV_ISSUE_URL      = "https://webapi.cninfo.com.cn/api/sysapi/p_sysapi1123"
	WEBAPI_BOND_COV_STOCK_ISSUE_URL = "https://webapi.cninfo.com.cn/api/sysapi/p_sysapi1124"
	WEBAPI_BOND_LOCAL_GOVERNMENT_ISSUE_URL = "https://webapi.cninfo.com.cn/api/sysapi/p_sysapi1121"
	WEBAPI_BOND_TREASURE_ISSUE_URL = "https://webapi.cninfo.com.cn/api/sysapi/p_sysapi1120"

	AES_KEY = []byte("1234567887654321")
	AES_IV  = []byte("1234567887654321")
)

type Adapter interface {
	Name() string
	Description() string
	Request(ctx context.Context, params map[string]interface{}) ([]map[string]interface{}, error)
}

// webapiField maps a Go field name to its source in the webapi response.
// The source is either an index into the row's value slice, or a key name.
type webapiField struct {
	name   string
	source string
	kind   string
}

// fieldSourceKey matches a _WebapiField's source field to the actual API response.
// Returns the value from the row, handling both numeric-index and named-key sources.
func fieldSourceKey(row map[string]interface{}, source string) interface{} {
	v, ok := row[source]
	if ok {
		return v
	}
	i, err := strconv.Atoi(source)
	if err == nil {
		values := make([]interface{}, 0, len(row))
		for _, val := range row {
			values = append(values, val)
		}
		sort.Slice(values, func(a, b int) bool {
			sa := fmt.Sprintf("%v", values[a])
			sb := fmt.Sprintf("%v", values[b])
			return sa < sb
		})
		if i >= 0 && i < len(values) {
			return values[i]
		}
	}
	return nil
}

// webapiRecords extracts the "records" array from a webapi response.
func webapiRecords(payload map[string]interface{}) []map[string]interface{} {
	if payload == nil {
		return nil
	}
	val, ok := payload["records"]
	if !ok {
		return nil
	}
	arr, ok := val.([]interface{})
	if !ok {
		return nil
	}
	results := make([]map[string]interface{}, 0, len(arr))
	for _, v := range arr {
		m, ok := v.(map[string]interface{})
		if ok {
			results = append(results, m)
		}
	}
	return results
}

// normalizeWebapiRow applies field mapping to a single webapi record.
func normalizeWebapiRow(row map[string]interface{}, fields []webapiField) map[string]interface{} {
	result := make(map[string]interface{})
	for _, f := range fields {
		raw := fieldSourceKey(row, f.source)
		val := normalizeWebapiValue(raw, f.kind)
		result[f.name] = val
	}
	return result
}

// normalizeWebapiValue parses a raw value according to its kind.
func normalizeWebapiValue(raw interface{}, kind string) interface{} {
	if raw == nil {
		return nil
	}
	s := cleanText(raw)
	if s == "" {
		return ""
	}
	switch kind {
	case "float":
		v, err := strconv.ParseFloat(strings.ReplaceAll(s, ",", ""), 64)
		if err != nil {
			return s
		}
		return v
	case "int":
		v, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return s
		}
		return int(v)
	case "date":
		d := normalizeDate(s)
		return d
	default:
		return s
	}
}

// webapiFetch sends a webapi request with the proper Accept-Enckey header.
func (a *CNINFOAdapter) webapiFetch(ctx context.Context, method string, u string, params url.Values, originOverride string) (map[string]interface{}, error) {
	origin := "https://webapi.cninfo.com.cn"
	if originOverride != "" {
		origin = originOverride
	}
	fullURL := u
	if len(params) > 0 {
		fullURL = u + "?" + params.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, fullURL, nil)
	if err != nil {
		return nil, fmt.Errorf("cninfo webapi request: %w", err)
	}
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Accept-Enckey", cninfoEnckey())
	req.Header.Set("Origin", origin)
	req.Header.Set("Referer", origin+"/")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	req.Header.Set("User-Agent", "Mozilla/5.0 AxData/0.1")

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cninfo webapi request: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("cninfo webapi request: %w", err)
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, fmt.Errorf("cninfo webapi response invalid JSON: %w", err)
	}

	resultCode, _ := payload["resultcode"].(string)
	if resultCode != "" && resultCode != "200" && resultCode != "0" {
		msg, _ := payload["resultmsg"]
		return nil, fmt.Errorf("cninfo webapi error code=%s msg=%v", resultCode, msg)
	}
	return payload, nil
}

// formFetch sends a form-urlencoded POST to cninfo.com.cn.
func (a *CNINFOAdapter) formFetch(ctx context.Context, method string, u string, data url.Values, headers map[string]string) (map[string]interface{}, error) {
	buf := strings.NewReader(data.Encode())
	req, err := http.NewRequestWithContext(ctx, method, u, buf)
	if err != nil {
		return nil, fmt.Errorf("cninfo form request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=UTF-8")
	req.Header.Set("Origin", "https://www.cninfo.com.cn")
	req.Header.Set("Referer", "https://www.cninfo.com.cn/")
	req.Header.Set("User-Agent", "Mozilla/5.0 AxData/0.1")
	req.Header.Set("Accept", "application/json,text/plain,*/*")
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cninfo form request: %w", err)
	}
	defer resp.Body.Close()
	dataBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("cninfo form request: %w", err)
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(dataBytes, &payload); err != nil {
		return nil, fmt.Errorf("cninfo form response invalid JSON: %w", err)
	}
	return payload, nil
}

// jsonFetch sends a generic GET request.
func (a *CNINFOAdapter) jsonFetch(ctx context.Context, u string, headers map[string]string) (map[string]interface{}, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 AxData/0.1")
	req.Header.Set("Accept", "application/json,text/plain,*/*")
	req.Header.Set("Referer", "https://www.cninfo.com.cn/")
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cninfo json request: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("cninfo json request: %w", err)
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, fmt.Errorf("cninfo json response invalid JSON: %w", err)
	}
	return payload, nil
}

// jsonFetchPOST sends a JSON POST request.
func (a *CNINFOAdapter) jsonFetchPOST(ctx context.Context, u string, body interface{}, headers map[string]string) (map[string]interface{}, error) {
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Mozilla/5.0 AxData/0.1")
	req.Header.Set("Referer", "https://www.cninfo.com.cn/")
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cninfo json post request: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("cninfo json post request: %w", err)
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, fmt.Errorf("cninfo json post response invalid JSON: %w", err)
	}
	return payload, nil
}

// CNINFOAdapter implements the CNINFO (巨潮) API.
type CNINFOAdapter struct {
	baseURL     string
	client      *http.Client
	description string
	stockIndex  map[string]map[string]interface{}
}

func NewCNINFOAdapter() *CNINFOAdapter {
	return &CNINFOAdapter{
		baseURL:     "https://www.cninfo.com.cn",
		client:      &http.Client{Timeout: 60 * time.Second},
		description: "CNINFO (巨潮) API adapter",
	}
}

// Name returns the adapter name.
func (a *CNINFOAdapter) Name() string {
	return "cninfo"
}

// Description returns the adapter description.
func (a *CNINFOAdapter) Description() string {
	return a.description
}

// Request dispatches to the correct handler by interface name.
func (a *CNINFOAdapter) Request(ctx context.Context, params map[string]interface{}) ([]map[string]interface{}, error) {
	interfaceName := paramString(params, "interface")
	switch interfaceName {
	case "cninfo_announcements":
		return a.requestAnnouncements(ctx, params)
	case "cninfo_announcement_detail":
		return a.requestAnnouncementDetail(ctx, params)
	case "stock_irm_cninfo":
		return a.requestIRMQuestions(ctx, params)
	case "stock_irm_ans_cninfo":
		return a.requestIRMAnswer(ctx, params)
	case "stock_zh_a_disclosure_report_cninfo":
		return a.requestDisclosure(ctx, params, "fulltext")
	case "stock_zh_a_disclosure_relation_cninfo":
		return a.requestDisclosure(ctx, params, "relation")
	case "stock_profile_cninfo":
		return a.requestWebapiSimple(ctx, params, WEBAPI_STOCK_PROFILE_URL, WEBAPI_STOCK_PROFILE_FIELDS)
	case "stock_allotment_cninfo":
		return a.requestWebapi(ctx, params, WEBAPI_STOCK_ALLOTMENT_URL, WEBAPI_STOCK_ALLOTMENT_FIELDS, buildAllotmentParams)
	case "stock_dividend_cninfo":
		return a.requestWebapiSimple(ctx, params, WEBAPI_STOCK_DIVIDEND_URL, WEBAPI_STOCK_DIVIDEND_FIELDS)
	case "stock_hold_change_cninfo":
		return a.requestWebapi(ctx, params, WEBAPI_STOCK_HOLD_CHANGE_URL, WEBAPI_STOCK_HOLD_CHANGE_FIELDS, buildHoldChangeParams)
	case "stock_hold_control_cninfo":
		return a.requestWebapi(ctx, params, WEBAPI_STOCK_HOLD_CONTROL_URL, WEBAPI_STOCK_HOLD_CONTROL_FIELDS, buildHoldControlParams)
	case "stock_hold_num_cninfo":
		return a.requestWebapi(ctx, params, WEBAPI_STOCK_HOLD_NUM_URL, WEBAPI_STOCK_HOLD_NUM_FIELDS, buildHoldNumParams)
	case "stock_industry_category_cninfo":
		return a.requestWebapi(ctx, params, WEBAPI_STOCK_INDUSTRY_CATEGORY_URL, WEBAPI_STOCK_INDUSTRY_CATEGORY_FIELDS, buildIndustryCategoryParams)
	case "stock_ipo_summary_cninfo":
		return a.requestWebapiSimple(ctx, params, WEBAPI_STOCK_IPO_SUMMARY_URL, WEBAPI_STOCK_IPO_SUMMARY_FIELDS)
	case "stock_new_gh_cninfo":
		return a.requestWebapiNoParams(ctx, params, WEBAPI_STOCK_NEW_GH_URL, WEBAPI_STOCK_NEW_GH_FIELDS)
	case "stock_new_ipo_cninfo":
		return a.requestWebapi(ctx, params, WEBAPI_STOCK_NEW_IPO_URL, WEBAPI_STOCK_NEW_IPO_FIELDS, buildNewIPOParams)
	case "stock_share_change_cninfo":
		return a.requestWebapi(ctx, params, WEBAPI_STOCK_SHARE_CHANGE_URL, WEBAPI_STOCK_SHARE_CHANGE_FIELDS, buildShareChangeParams)
	case "fund_report_asset_allocation_cninfo":
		return a.requestWebapiNoParams(ctx, params, WEBAPI_FUND_ASSET_ALLOCATION_URL, WEBAPI_FUND_ASSET_ALLOCATION_FIELDS)
	case "fund_report_industry_allocation_cninfo":
		return a.requestWebapi(ctx, params, WEBAPI_FUND_INDUSTRY_ALLOCATION_URL, WEBAPI_FUND_INDUSTRY_ALLOCATION_FIELDS, buildFundIndustryAllocationParams)
	case "fund_report_stock_cninfo":
		return a.requestWebapi(ctx, params, WEBAPI_FUND_STOCK_URL, WEBAPI_FUND_STOCK_FIELDS, buildFundStockParams)
	case "stock_cg_equity_mortgage_cninfo":
		return a.requestWebapi(ctx, params, WEBAPI_STOCK_CG_EQUITY_MORTGAGE_URL, WEBAPI_STOCK_CG_EQUITY_MORTGAGE_FIELDS, buildEquityMortgageParams)
	case "stock_cg_guarantee_cninfo":
		return a.requestWebapi(ctx, params, WEBAPI_STOCK_CG_GUARANTEE_URL, WEBAPI_STOCK_CG_GUARANTEE_FIELDS, buildGuaranteeParams)
	case "stock_cg_lawsuit_cninfo":
		return a.requestWebapi(ctx, params, WEBAPI_STOCK_CG_LAWSUIT_URL, WEBAPI_STOCK_CG_LAWSUIT_FIELDS, buildLawsuitParams)
	case "stock_hold_management_detail_cninfo":
		return a.requestWebapi(ctx, params, WEBAPI_STOCK_HOLD_MANAGEMENT_DETAIL_URL, WEBAPI_STOCK_HOLD_MANAGEMENT_DETAIL_FIELDS, buildHoldManagementDetailParams)
	case "stock_industry_change_cninfo":
		return a.requestWebapi(ctx, params, WEBAPI_STOCK_INDUSTRY_CHANGE_URL, WEBAPI_STOCK_INDUSTRY_CHANGE_FIELDS, buildIndustryChangeParams)
	case "stock_industry_pe_ratio_cninfo":
		return a.requestWebapi(ctx, params, WEBAPI_STOCK_INDUSTRY_PE_RATIO_URL, WEBAPI_STOCK_INDUSTRY_PE_RATIO_FIELDS, buildIndustryPERatioParams)
	case "stock_rank_forecast_cninfo":
		return a.requestWebapi(ctx, params, WEBAPI_STOCK_RANK_FORECAST_URL, WEBAPI_STOCK_RANK_FORECAST_FIELDS, buildRankForecastParams)
	case "bond_corporate_issue_cninfo":
		return a.requestWebapi(ctx, params, WEBAPI_BOND_CORPORATE_ISSUE_URL, WEBAPI_BOND_CORPORATE_ISSUE_FIELDS, buildBondDateRangeParams)
	case "bond_cov_issue_cninfo":
		return a.requestWebapi(ctx, params, WEBAPI_BOND_COV_ISSUE_URL, WEBAPI_BOND_COV_ISSUE_FIELDS, buildBondDateRangeParams)
	case "bond_cov_stock_issue_cninfo":
		return a.requestWebapiNoParams(ctx, params, WEBAPI_BOND_COV_STOCK_ISSUE_URL, WEBAPI_BOND_COV_STOCK_ISSUE_FIELDS)
	case "bond_local_government_issue_cninfo":
		return a.requestWebapi(ctx, params, WEBAPI_BOND_LOCAL_GOVERNMENT_ISSUE_URL, WEBAPI_BOND_PUBLIC_ISSUE_FIELDS, buildBondDateRangeParams)
	case "bond_treasure_issue_cninfo":
		return a.requestWebapi(ctx, params, WEBAPI_BOND_TREASURE_ISSUE_URL, WEBAPI_BOND_PUBLIC_ISSUE_FIELDS, buildBondDateRangeParams)
	default:
		return nil, fmt.Errorf("cninfo: unknown interface %s", interfaceName)
	}
}

// buildDateRangeParams returns sdate/edate from params.
func buildDateRangeParams(params map[string]interface{}, defaults map[string]string) url.Values {
	qs := url.Values{}
	sdate := paramString(params, "start_date")
	if sdate == "" {
		sdate = defaults["start_date"]
	}
	edate := paramString(params, "end_date")
	if edate == "" {
		edate = defaults["end_date"]
	}
	qs.Set("sdate", dateDash(sdate))
	qs.Set("edate", dateDash(edate))
	return qs
}

func buildAllotmentParams(params map[string]interface{}) url.Values {
	qs := url.Values{}
	if sym := symbolFromCode(paramString(params, "code")); sym != "" {
		qs.Set("scode", sym)
	}
	dr := buildDateRangeParams(params, map[string]string{"start_date": "19900101", "end_date": "20991231"})
	for k, v := range dr {
		qs[k] = v
	}
	return qs
}

func buildHoldChangeParams(params map[string]interface{}) url.Values {
	qs := url.Values{}
	market, ok := cninfoHoldChangeMarketMap[paramString(params, "market")]
	if !ok {
		market = cninfoHoldChangeMarketMap["沪市"]
	}
	qs.Set("market", market)
	return qs
}

func buildHoldControlParams(params map[string]interface{}) url.Values {
	qs := url.Values{}
	ct, ok := cninfoHoldControlTypeMap[paramString(params, "control_type")]
	if !ok {
		ct = cninfoHoldControlTypeMap["实际控制人"]
	}
	qs.Set("ctype", ct)
	return qs
}

func buildHoldNumParams(params map[string]interface{}) url.Values {
	qs := url.Values{}
	rdate := paramString(params, "date")
	if rdate == "" {
		rdate = "20210630"
	}
	qs.Set("rdate", rdate)
	return qs
}

func buildIndustryCategoryParams(params map[string]interface{}) url.Values {
	qs := url.Values{}
	indType, ok := cninfoIndustryTypeMap[paramString(params, "industry_type")]
	if !ok {
		indType = cninfoIndustryTypeMap["证监会行业分类标准"]
	}
	qs.Set("indcode", "")
	qs.Set("indtype", indType)
	qs.Set("format", "json")
	return qs
}

func buildNewIPOParams(params map[string]interface{}) url.Values {
	qs := url.Values{}
	tt := paramString(params, "time_type")
	if tt == "" {
		tt = "36"
	}
	market := strings.ToUpper(paramString(params, "market"))
	if market == "" {
		market = "ALL"
	}
	qs.Set("timetype", tt)
	qs.Set("market", market)
	return qs
}

func buildShareChangeParams(params map[string]interface{}) url.Values {
	qs := url.Values{}
	if sym := symbolFromCode(paramString(params, "code")); sym != "" {
		qs.Set("scode", sym)
	}
	dr := buildDateRangeParams(params, map[string]string{"start_date": "19900101", "end_date": "20991231"})
	for k, v := range dr {
		qs[k] = v
	}
	return qs
}

func buildFundIndustryAllocationParams(params map[string]interface{}) url.Values {
	qs := url.Values{}
	rdate := paramString(params, "date")
	if rdate == "" {
		rdate = "20210630"
	}
	qs.Set("rdate", dateDash(rdate))
	return qs
}

func buildFundStockParams(params map[string]interface{}) url.Values {
	qs := url.Values{}
	rdate := paramString(params, "date")
	if rdate == "" {
		rdate = "20210630"
	}
	qs.Set("rdate", dateDash(rdate))
	return qs
}

func buildEquityMortgageParams(params map[string]interface{}) url.Values {
	qs := url.Values{}
	tdate := paramString(params, "date")
	if tdate == "" {
		tdate = "20210930"
	}
	qs.Set("tdate", dateDash(tdate))
	return qs
}

func buildGuaranteeParams(params map[string]interface{}) url.Values {
	qs := url.Values{}
	marketCode, ok := cninfoHoldChangeMarketMap[paramString(params, "market")]
	if !ok {
		marketCode = cninfoHoldChangeMarketMap["全部"]
	}
	dr := buildDateRangeParams(params, map[string]string{"start_date": "20180630", "end_date": "20210927"})
	for k, v := range dr {
		qs[k] = v
	}
	qs.Set("market", marketCode)
	return qs
}

func buildLawsuitParams(params map[string]interface{}) url.Values {
	qs := url.Values{}
	marketCode, ok := cninfoHoldChangeMarketMap[paramString(params, "market")]
	if !ok {
		marketCode = cninfoHoldChangeMarketMap["沪市"]
	}
	dr := buildDateRangeParams(params, map[string]string{"start_date": "20180630", "end_date": "20210927"})
	for k, v := range dr {
		qs[k] = v
	}
	qs.Set("market", marketCode)
	return qs
}

func buildHoldManagementDetailParams(params map[string]interface{}) url.Values {
	qs := url.Values{}
	ct, ok := cninfoManagementChangeTypeMap[paramString(params, "change_type")]
	if !ok {
		ct = cninfoManagementChangeTypeMap["增持"]
	}
	dr := buildDateRangeParams(params, map[string]string{"start_date": "20240101", "end_date": "20241231"})
	for k, v := range dr {
		qs[k] = v
	}
	qs.Set("varytype", ct)
	return qs
}

func buildIndustryChangeParams(params map[string]interface{}) url.Values {
	qs := url.Values{}
	if sym := symbolFromCode(paramString(params, "code")); sym != "" {
		qs.Set("scode", sym)
	}
	dr := buildDateRangeParams(params, map[string]string{"start_date": "20091227", "end_date": "20220713"})
	for k, v := range dr {
		qs[k] = v
	}
	return qs
}

func buildIndustryPERatioParams(params map[string]interface{}) url.Values {
	qs := url.Values{}
	sortCode, ok := cninfoIndustryPEClassMap[paramString(params, "classification")]
	if !ok {
		sortCode = cninfoIndustryPEClassMap["证监会行业分类"]
	}
	tdate := paramString(params, "date")
	if tdate == "" {
		tdate = "20240617"
	}
	qs.Set("tdate", dateDash(tdate))
	qs.Set("sortcode", sortCode)
	return qs
}

func buildRankForecastParams(params map[string]interface{}) url.Values {
	qs := url.Values{}
	tdate := paramString(params, "date")
	if tdate == "" {
		tdate = "20230817"
	}
	qs.Set("tdate", dateDash(tdate))
	return qs
}

func buildBondDateRangeParams(params map[string]interface{}) url.Values {
	dr := buildDateRangeParams(params, map[string]string{"start_date": "20210911", "end_date": "20211110"})
	return dr
}

// requestWebapi handles a standard webapi with field mapping.
func (a *CNINFOAdapter) requestWebapi(ctx context.Context, params map[string]interface{}, apiURL string, fields []webapiField, buildParams func(map[string]interface{}) url.Values) ([]map[string]interface{}, error) {
	qs := buildParams(params)
	payload, err := a.webapiFetch(ctx, "POST", apiURL, qs, "")
	if err != nil {
		return nil, err
	}
	records := webapiRecords(payload)
	if records == nil {
		return nil, fmt.Errorf("cninfo webapi returned no records")
	}
	results := make([]map[string]interface{}, 0, len(records))
	for _, row := range records {
		m := normalizeWebapiRow(row, fields)
		sym := symbolFromCode(paramString(row, "symbol"))
		if sym == "" {
			sym = paramString(row, "symbol")
		}
		if sym != "" {
			m["instrument_id"] = instrumentIDFromSymbol(sym)
			m["exchange"] = exchangeFromSymbol(sym)
			m["symbol"] = sym
		}
		results = append(results, m)
	}
	return results, nil
}

// requestWebapiSimple handles webapi with just scode parameter.
func (a *CNINFOAdapter) requestWebapiSimple(ctx context.Context, params map[string]interface{}, apiURL string, fields []webapiField) ([]map[string]interface{}, error) {
	return a.requestWebapi(ctx, params, apiURL, fields, func(p map[string]interface{}) url.Values {
		qs := url.Values{}
		if sym := symbolFromCode(paramString(p, "code")); sym != "" {
			qs.Set("scode", sym)
		}
		return qs
	})
}

// requestWebapiNoParams handles webapi with no parameters.
func (a *CNINFOAdapter) requestWebapiNoParams(ctx context.Context, params map[string]interface{}, apiURL string, fields []webapiField) ([]map[string]interface{}, error) {
	return a.requestWebapi(ctx, params, apiURL, fields, func(p map[string]interface{}) url.Values {
		return url.Values{}
	})
}

// requestAnnouncements fetches announcements for a code.
func (a *CNINFOAdapter) requestAnnouncements(ctx context.Context, params map[string]interface{}) ([]map[string]interface{}, error) {
	code := paramString(params, "code")
	symbol := symbolFromCode(code)
	if symbol == "" {
		return nil, fmt.Errorf("cninfo: code is required")
	}
	stock, err := a.resolveStock(ctx, symbol)
	if err != nil {
		return nil, fmt.Errorf("cninfo: resolve stock: %w", err)
	}
	if stock == nil {
		return nil, fmt.Errorf("cninfo: stock not found for %s", symbol)
	}

	page := paramInt(params, "page", 1)
	limit := paramInt(params, "limit", 30)
	if limit > 100 {
		limit = 100
	}

	startDate := paramString(params, "start_date")
	endDate := paramString(params, "end_date")

	orgID := stock["orgId"]
	orgIDStr := ""
	if oid, ok := orgID.(string); ok {
		orgIDStr = oid
	}

	data := url.Values{
		"pageNum":   {strconv.Itoa(page)},
		"pageSize":  {strconv.Itoa(limit)},
		"column":    {cninfoColumn(symbol)},
		"tabName":   {"fulltext"},
		"plate":     {cninfoPlate(symbol)},
		"stock":     {fmt.Sprintf("%s,%s", symbol, orgIDStr)},
		"searchkey": {""},
		"secid":     {""},
		"category":  {""},
		"trade":     {""},
		"seDate":    {cninfoDateRange(startDate, endDate)},
		"sortName":  {""},
		"sortType":  {""},
		"isHLtitle": {"true"},
	}

	payload, err := a.formFetch(ctx, "POST", CNINFO_ANNOUNCEMENT_QUERY_URL, data, map[string]string{})
	if err != nil {
		return nil, err
	}

	announcements, ok := payload["announcements"]
	if !ok {
		announcements, ok = payload["annList"]
		if !ok {
			return nil, fmt.Errorf("cninfo: no announcements in response")
		}
	}
	arr, ok := announcements.([]interface{})
	if !ok {
		return nil, fmt.Errorf("cninfo: announcements is not a list")
	}

	results := make([]map[string]interface{}, 0, len(arr))
	for _, item := range arr {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		results = append(results, normalizeAnnouncementRow(m, stock))
	}
	return results, nil
}

// requestAnnouncementDetail fetches PDF metadata for a single announcement.
func (a *CNINFOAdapter) requestAnnouncementDetail(ctx context.Context, params map[string]interface{}) ([]map[string]interface{}, error) {
	adjunctURL := paramString(params, "url")
	if adjunctURL == "" {
		adjunctURL = paramString(params, "download_url")
	}
	announcementID := paramString(params, "announcement_id")

	if adjunctURL == "" {
		return nil, fmt.Errorf("cninfo: url is required")
	}

	downloadURL := cninfoDownloadURL(adjunctURL)
	metadata := a.fetchPDFMetadata(ctx, downloadURL)

	result := map[string]interface{}{
		"announcement_id": announcementID,
		"title":           paramString(params, "title"),
		"content_type":    metadata["content_type"],
		"file_size_bytes": metadata["file_size_bytes"],
		"download_url":    downloadURL,
	}
	return []map[string]interface{}{result}, nil
}

// requestIRMQuestions fetches IRM questions.
func (a *CNINFOAdapter) requestIRMQuestions(ctx context.Context, params map[string]interface{}) ([]map[string]interface{}, error) {
	symbol := symbolFromCode(paramString(params, "code"))
	if symbol == "" {
		return nil, fmt.Errorf("cninfo: code is required for IRM")
	}

	orgID, err := a.fetchIRMOrgID(ctx, symbol)
	if err != nil {
		return nil, err
	}

	page := paramInt(params, "page", 1)
	limit := paramInt(params, "limit", 30)
	if limit > 1000 {
		limit = 1000
	}

	startDate := paramString(params, "start_date")
	endDate := paramString(params, "end_date")
	keyword := paramString(params, "keyword")

	query := url.Values{
		"_t":      {"1691142650"},
		"stockcode": {symbol},
		"orgId":   {orgID},
		"pageSize":  {strconv.Itoa(limit)},
		"pageNum":   {strconv.Itoa(page)},
		"keyWord":   {keyword},
		"startDay":  {dateDash(startDate)},
		"endDay":    {dateDash(endDate)},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, CNINFO_IRM_QUESTION_URL+"?"+query.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Referer", "https://irm.cninfo.com.cn/")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "Mozilla/5.0 AxData/0.1")
	req.Header.Set("Accept", "application/json,text/plain,*/*")

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cninfo IRM request: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("cninfo IRM request: %w", err)
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, fmt.Errorf("cninfo IRM response invalid JSON: %w", err)
	}

	rows := payload["rows"]
	arr, ok := rows.([]interface{})
	if !ok {
		arr = []interface{}{}
	}

	results := make([]map[string]interface{}, 0, len(arr))
	for _, item := range arr {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		results = append(results, normalizeIRMQuestionRow(m, symbol))
	}
	return results, nil
}

// requestIRMAnswer fetches an IRM question detail/answer.
func (a *CNINFOAdapter) requestIRMAnswer(ctx context.Context, params map[string]interface{}) ([]map[string]interface{}, error) {
	questionID := paramString(params, "question_id")
	if questionID == "" {
		return nil, fmt.Errorf("cninfo: question_id is required")
	}

	qs := url.Values{"questionId": {questionID}, "_t": {"1691146921"}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, CNINFO_IRM_DETAIL_URL+"?"+qs.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Referer", "https://irm.cninfo.com.cn/")
	req.Header.Set("User-Agent", "Mozilla/5.0 AxData/0.1")

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cninfo IRM detail request: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("cninfo IRM detail request: %w", err)
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, fmt.Errorf("cninfo IRM detail response invalid JSON: %w", err)
	}

	dataMap, ok := payload["data"]
	if !ok {
		return []map[string]interface{}{}, nil
	}
	dm, ok := dataMap.(map[string]interface{})
	if !ok {
		return []map[string]interface{}{}, nil
	}

	result := normalizeIRMAnswerRow(dm, questionID)
	if result == nil {
		return []map[string]interface{}{}, nil
	}
	return []map[string]interface{}{result}, nil
}

// requestDisclosure fetches disclosure reports/relations.
func (a *CNINFOAdapter) requestDisclosure(ctx context.Context, params map[string]interface{}, tabName string) ([]map[string]interface{}, error) {
	symbol := symbolFromCode(paramString(params, "code"))
	if symbol == "" {
		return nil, fmt.Errorf("cninfo: code is required for disclosure")
	}
	stock, err := a.resolveStock(ctx, symbol)
	if err != nil {
		return nil, fmt.Errorf("cninfo: resolve stock: %w", err)
	}
	if stock == nil {
		return nil, fmt.Errorf("cninfo: stock not found for %s", symbol)
	}

	page := paramInt(params, "page", 1)
	limit := paramInt(params, "limit", 30)
	if limit > 100 {
		limit = 100
	}

	category := paramDisclosureCategory(paramString(params, "category"))
	startDate := paramString(params, "start_date")
	endDate := paramString(params, "end_date")

	orgID := ""
	if oid, ok := stock["orgId"]; ok {
		orgID, _ = oid.(string)
	}

	data := url.Values{
		"pageNum":   {strconv.Itoa(page)},
		"pageSize":  {strconv.Itoa(limit)},
		"column":    {"szse"},
		"tabName":   {tabName},
		"plate":     {""},
		"stock":     {fmt.Sprintf("%s,%s", symbol, orgID)},
		"searchkey": {paramString(params, "keyword")},
		"secid":     {""},
		"category":  {category},
		"trade":     {""},
		"seDate":    {cninfoDateRange(startDate, endDate)},
		"sortName":  {""},
		"sortType":  {""},
		"isHLtitle": {"true"},
	}

	payload, err := a.formFetch(ctx, "POST", CNINFO_ANNOUNCEMENT_QUERY_URL, data, map[string]string{})
	if err != nil {
		return nil, err
	}

	rawRows, ok := payload["announcements"]
	if !ok {
		return []map[string]interface{}{}, nil
	}
	arr, ok := rawRows.([]interface{})
	if !ok {
		return []map[string]interface{}{}, nil
	}

	results := make([]map[string]interface{}, 0, len(arr))
	for _, item := range arr {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		results = append(results, normalizeAnnouncementRow(m, stock))
	}
	return results, nil
}

// resolveStock looks up stock metadata from the stock index.
func (a *CNINFOAdapter) resolveStock(ctx context.Context, symbol string) (map[string]interface{}, error) {
	if a.stockIndex == nil {
		if err := a.loadStockIndex(ctx); err != nil {
			return nil, err
		}
	}
	stock, ok := a.stockIndex[symbol]
	if !ok {
		return nil, nil
	}
	result := map[string]interface{}{
		"symbol":      symbol,
		"instrument_id": instrumentIDFromSymbol(symbol),
		"exchange":     exchangeFromSymbol(symbol),
		"orgId":        stock["orgId"],
		"name":         stock["zwjc"],
	}
	return result, nil
}

// loadStockIndex fetches and caches the stock index.
func (a *CNINFOAdapter) loadStockIndex(ctx context.Context) error {
	payload, err := a.jsonFetch(ctx, CNINFO_STOCK_INDEX_URL, nil)
	if err != nil {
		return err
	}

	stockList, ok := payload["stockList"]
	if !ok {
		return fmt.Errorf("cninfo: stock index missing stockList")
	}
	arr, ok := stockList.([]interface{})
	if !ok {
		return fmt.Errorf("cninfo: stockList is not a list")
	}

	index := make(map[string]map[string]interface{})
	for _, item := range arr {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		code, ok := m["code"]
		if !ok {
			continue
		}
		index[code.(string)] = m
	}
	a.stockIndex = index
	return nil
}

// fetchIRMOrgID looks up the IRM org ID for a symbol.
func (a *CNINFOAdapter) fetchIRMOrgID(ctx context.Context, symbol string) (string, error) {
	data := url.Values{"keyWord": {symbol}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, CNINFO_IRM_KEYWORD_URL+"?_t=1691144074", strings.NewReader(data.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Referer", "https://irm.cninfo.com.cn/")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "Mozilla/5.0 AxData/0.1")
	req.Header.Set("Accept", "application/json,text/plain,*/*")

	resp, err := a.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("cninfo IRM org lookup: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", err
	}

	dataList, ok := payload["data"]
	if !ok {
		return "", fmt.Errorf("cninfo IRM: no data for %s", symbol)
	}
	arr, ok := dataList.([]interface{})
	if !ok || len(arr) == 0 {
		return "", fmt.Errorf("cninfo IRM: no org id for %s", symbol)
	}
	first, ok := arr[0].(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("cninfo IRM: invalid data for %s", symbol)
	}
	secid, ok := first["secid"]
	if !ok {
		return "", fmt.Errorf("cninfo IRM: missing secid for %s", symbol)
	}
	return secid.(string), nil
}

// fetchPDFMetadata fetches HEAD request metadata for a PDF URL.
func (a *CNINFOAdapter) fetchPDFMetadata(ctx context.Context, u string) map[string]interface{} {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, u, nil)
	if err != nil {
		return map[string]interface{}{"error": fmt.Sprintf("request error: %v", err)}
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 AxData/0.1")
	req.Header.Set("Accept", "application/pdf,*/*")
	req.Header.Set("Range", "bytes=0-0")
	req.Header.Set("Referer", "https://www.cninfo.com.cn/")

	resp, err := a.client.Do(req)
	if err != nil {
		return map[string]interface{}{"error": fmt.Sprintf("head request failed: %v", err)}
	}
	defer resp.Body.Close()

	contentType := resp.Header.Get("Content-Type")
	contentRange := resp.Header.Get("Content-Range")
	contentLength := resp.Header.Get("Content-Length")

	fileSize := fileSizeFromHeaders(contentRange, contentLength)

	return map[string]interface{}{
		"content_type":    contentType,
		"file_size_bytes": fileSize,
	}
}

// cninfoEnckey generates the AES-encrypted enckey header value.
func cninfoEnckey() string {
	nowStr := strconv.FormatInt(time.Now().Unix(), 10)
	plaintext := []byte(nowStr)
	padded := pkcs7Pad(plaintext, 16)

	c, err := aes.NewCipher(AES_KEY)
	if err != nil {
		return ""
	}
	mode := cipher.NewCBCEncrypter(c, AES_IV)
	ciphertext := make([]byte, len(padded))
	mode.CryptBlocks(ciphertext, padded)

	return base64.StdEncoding.EncodeToString(ciphertext)
}

// pkcs7Pad applies PKCS7 padding.
func pkcs7Pad(data []byte, blockSize int) []byte {
	padding := blockSize - len(data)%blockSize
	padText := bytes.Repeat([]byte{byte(padding)}, padding)
	return append(data, padText...)
}

// normalizeAnnouncementRow maps an announcement item to a flat dict.
func normalizeAnnouncementRow(row map[string]interface{}, stock map[string]interface{}) map[string]interface{} {
	adjunctURL := paramString(row, "adjunctUrl")
	title := cleanText(row["announcementTitle"])
	if title == "" {
		title = cleanText(row["shortTitle"])
	}

	publishDate := timestampMSToDate(row["announcementTime"])
	adjunctSize := parseFloat(row["adjunctSize"])

	result := map[string]interface{}{
		"instrument_id":   stock["instrument_id"],
		"symbol":          stock["symbol"],
		"exchange":        stock["exchange"],
		"name":            cleanText(row["secName"]),
		"announcement_id": cleanText(row["announcementId"]),
		"title":           title,
		"publish_date":    publishDate,
		"file_type":       cleanText(row["adjunctType"]),
		"file_size_kb":    adjunctSize,
		"download_url":    cninfoDownloadURL(adjunctURL),
	}
	return result
}

// normalizeIRMQuestionRow maps an IRM question item.
func normalizeIRMQuestionRow(row map[string]interface{}, symbol string) map[string]interface{} {
	secid := paramString(row, "secid")
	result := map[string]interface{}{
		"instrument_id": instrumentIDFromSymbol(secid),
		"symbol":        secid,
		"exchange":      exchangeFromSymbol(secid),
		"question_id":   cleanText(row["indexId"]),
		"question":      cleanText(row["mainContent"]),
		"questioner":    cleanText(row["authorName"]),
		"questioner_id": cleanText(row["author"]),
		"answer_id":     cleanText(row["attachedId"]),
		"answer":        cleanText(row["attachedContent"]),
		"answerer":      cleanText(row["attachedAuthor"]),
		"question_time": timestampMSToDateTime(row["pubDate"]),
		"update_time":   timestampMSToDateTime(row["updateDate"]),
	}
	return result
}

// normalizeIRMAnswerRow maps an IRM answer detail.
func normalizeIRMAnswerRow(row map[string]interface{}, questionID string) map[string]interface{} {
	stockCode := paramString(row, "stockCode")
	symbol := symbolFromCode(stockCode)
	if symbol == "" {
		return nil
	}
	return map[string]interface{}{
		"instrument_id": instrumentIDFromSymbol(symbol),
		"symbol":        symbol,
		"exchange":      exchangeFromSymbol(symbol),
		"question_id":   questionID,
		"question":      cleanText(row["questionContent"]),
		"answer":        cleanText(row["replyContent"]),
		"questioner":    cleanText(row["questioner"]),
		"question_time": timestampMSToDateTime(row["questionDate"]),
		"answer_time":   timestampMSToDateTime(row["replyDate"]),
	}
}

// symbolFromCode extracts a 6-digit symbol from various code formats.
func symbolFromCode(value string) string {
	text := strings.TrimSpace(strings.ToUpper(value))
	text = strings.TrimSuffix(text, ".SH")
	text = strings.TrimSuffix(text, ".SZ")
	text = strings.TrimSuffix(text, ".BJ")
	if len(text) >= 8 {
		if text[:2] == "SH" || text[:2] == "SZ" || text[:2] == "BJ" {
			text = text[2:]
		}
	}
	if len(text) >= 6 && len(text) <= 6 {
		if _, err := strconv.Atoi(text); err == nil {
			return text
		}
	}
	return ""
}

// exchangeFromSymbol returns the exchange code for a symbol.
func exchangeFromSymbol(symbol string) string {
	if len(symbol) >= 1 {
		switch symbol[0] {
		case '6', '9', '5':
			return "SSE"
		case '4', '8':
			return "BSE"
		}
	}
	return "SZSE"
}

// instrumentIDFromSymbol returns instrument_id from symbol.
func instrumentIDFromSymbol(symbol string) string {
	suffix := "SZ"
	switch exchangeFromSymbol(symbol) {
	case "SSE":
		suffix = "SH"
	case "BSE":
		suffix = "BJ"
	}
	return symbol + "." + suffix
}

// cninfoColumn returns the cninfo column code for a symbol.
func cninfoColumn(symbol string) string {
	switch exchangeFromSymbol(symbol) {
	case "BSE":
		return "bj"
	case "SSE":
		return "sse"
	default:
		return "szse"
	}
}

// cninfoPlate returns the cninfo plate for a symbol.
func cninfoPlate(symbol string) string {
	switch exchangeFromSymbol(symbol) {
	case "BSE":
		return "bj"
	case "SSE":
		return "sh"
	default:
		return "sz"
	}
}

// cninfoDateRange formats a date range for cninfo.
func cninfoDateRange(startDate, endDate string) string {
	if startDate == "" && endDate == "" {
		return ""
	}
	return dateDash(startDate) + "~" + dateDash(endDate)
}

// dateDash converts YYYYMMDD to YYYY-MM-DD.
func dateDash(dateStr string) string {
	if dateStr == "" {
		return ""
	}
	s := strings.ReplaceAll(dateStr, "-", "")
	if len(s) >= 8 {
		return s[:4] + "-" + s[4:6] + "-" + s[6:8]
	}
	return dateStr
}

// cninfoDownloadURL formats a download URL.
func cninfoDownloadURL(value string) string {
	text := strings.TrimSpace(value)
	if strings.HasPrefix(text, "http://") || strings.HasPrefix(text, "https://") {
		return strings.ReplaceAll(text, "http://static.cninfo.com.cn/", CNINFO_STATIC_BASE)
	}
	return CNINFO_STATIC_BASE + strings.TrimLeft(text, "/")
}

// fileSizeFromHeaders parses file size from headers.
func fileSizeFromHeaders(contentRange, contentLength string) interface{} {
	if contentRange != "" {
		match := regexp.MustCompile(`/(\d+)$`).FindStringSubmatch(contentRange)
		if len(match) > 1 {
			v, err := strconv.Atoi(match[1])
			if err == nil {
				return v
			}
		}
	}
	if contentLength != "" {
		v, err := strconv.Atoi(contentLength)
		if err == nil {
			return v
		}
	}
	return nil
}

// parseFloat parses a float value.
func parseFloat(value interface{}) interface{} {
	if value == nil {
		return nil
	}
	s := cleanText(value)
	if s == "" {
		return nil
	}
	v, err := strconv.ParseFloat(strings.ReplaceAll(s, ",", ""), 64)
	if err != nil {
		return s
	}
	return v
}

// cleanText removes HTML tags and extra whitespace.
func cleanText(value interface{}) string {
	if value == nil {
		return ""
	}
	text := fmt.Sprintf("%v", value)
	text = regexp.MustCompile(`<[^>]+>`).ReplaceAllString(text, "")
	text = regexp.MustCompile(`\s+`).ReplaceAllString(text, " ")
	return strings.TrimSpace(text)
}

// paramString extracts a string param with type assertion.
func paramString(params map[string]interface{}, key string) string {
	if v, ok := params[key]; ok {
		return fmt.Sprintf("%v", v)
	}
	return ""
}

// paramInt extracts an int param with default.
func paramInt(params map[string]interface{}, key string, def int) int {
	if v, ok := params[key]; ok {
		switch iv := v.(type) {
		case int:
			return iv
		case int64:
			return int(iv)
		case float64:
			return int(iv)
		case string:
			i, err := strconv.Atoi(iv)
			if err == nil {
				return i
			}
		}
	}
	return def
}

// timestampMSToDate converts millisecond timestamp to YYYYMMDD.
func timestampMSToDate(value interface{}) string {
	if value == nil {
		return ""
	}
	var ms int64
	switch v := value.(type) {
	case int:
		ms = int64(v)
	case int64:
		ms = v
	case float64:
		ms = int64(v)
	case string:
		i, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return ""
		}
		ms = i
	default:
		return ""
	}
	sec := time.Unix(ms/1000, 0).In(time.FixedZone("CST", 8*3600))
	return sec.Format("20060102")
}

// timestampMSToDateTime converts millisecond timestamp to YYYYMMDDHHMMSS.
func timestampMSToDateTime(value interface{}) string {
	if value == nil {
		return ""
	}
	var ms int64
	switch v := value.(type) {
	case int:
		ms = int64(v)
	case int64:
		ms = v
	case float64:
		ms = int64(v)
	case string:
		i, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return ""
		}
		ms = i
	}
	sec := time.Unix(ms/1000, 0).In(time.FixedZone("CST", 8*3600))
	return sec.Format("20060102150405")
}

// normalizeDate extracts YYYYMMDD from a value.
func normalizeDate(value interface{}) string {
	if value == nil {
		return ""
	}
	s := cleanText(value)
	digits := regexp.MustCompile(`\D`).ReplaceAllString(s, "")
	if len(digits) >= 8 {
		candidate := digits[:8]
		_, err := time.Parse("20060102", candidate)
		if err == nil {
			return candidate
		}
	}
	return ""
}

// paramDisclosureCategory maps category names to their codes.
func paramDisclosureCategory(value string) string {
	if value == "" {
		return ""
	}
	if m, ok := cninfoDisclosureCategoryMap[value]; ok {
		return m
	}
	return value
}

var cninfoDisclosureCategoryMap = map[string]string{
	"年报":             "category_ndbg_szsh",
	"半年报":           "category_bndbg_szsh",
	"一季报":           "category_yjdbg_szsh",
	"三季报":           "category_sjdbg_szsh",
	"业绩预告":         "category_yjygjxz_szsh",
	"权益分派":         "category_qyfpxzcs_szsh",
	"董事会":           "category_dshgg_szsh",
	"监事会":           "category_jshgg_szsh",
	"股东大会":         "category_gddh_szsh",
	"日常经营":         "category_rcjy_szsh",
	"公司治理":         "category_gszl_szsh",
	"中介报告":         "category_zj_szsh",
	"首发":             "category_sf_szsh",
	"增发":             "category_zf_szsh",
	"股权激励":         "category_gqjl_szsh",
	"配股":             "category_pg_szsh",
	"解禁":             "category_jj_szsh",
	"公司债":           "category_gszq_szsh",
	"可转债":           "category_kzzq_szsh",
	"其他融资":         "category_qtrz_szsh",
	"股权变动":         "category_gqbd_szsh",
	"补充更正":         "category_bcgz_szsh",
	"澄清致歉":         "category_cqdq_szsh",
	"风险提示":         "category_fxts_szsh",
	"特别处理和退市":     "category_tbclts_szsh",
	"退市整理期":       "category_tszlq_szsh",
}

var cninfoHoldChangeMarketMap = map[string]string{
	"深市主板": "012002",
	"沪市":     "012001",
	"创业板":   "012015",
	"科创板":   "012029",
	"北交所":   "012046",
	"全部":     "",
}

var cninfoHoldControlTypeMap = map[string]string{
	"单独控制": "069001",
	"实际控制人": "069002",
	"一致行动人": "069003",
	"家族控制": "069004",
	"全部":     "",
}

var cninfoIndustryTypeMap = map[string]string{
	"证监会行业分类标准": "008001",
	"巨潮行业分类标准":   "008002",
	"申银万国行业分类标准": "008003",
	"新财富行业分类标准":   "008004",
	"国资委行业分类标准":   "008005",
	"巨潮产业细分标准":     "008006",
	"天相行业分类标准":     "008007",
	"全球行业分类标准":     "008008",
}

var cninfoManagementChangeTypeMap = map[string]string{
	"增持": "B",
	"减持": "S",
	"B":    "B",
	"S":    "S",
}

var cninfoIndustryPEClassMap = map[string]string{
	"证监会行业分类": "008001",
	"国证行业分类":   "008200",
	"008001":        "008001",
	"008200":        "008200",
}

// FIELD DEFINITIONS

// WEBAPI_STOCK_PROFILE_FIELDS
var WEBAPI_STOCK_PROFILE_FIELDS = []webapiField{
	{"company_name", "0", ""},
	{"english_name", "1", ""},
	{"former_short_name", "2", ""},
	{"a_share_code", "3", ""},
	{"a_share_name", "4", ""},
	{"b_share_code", "5", ""},
	{"b_share_name", "6", ""},
	{"h_share_code", "7", ""},
	{"h_share_name", "8", ""},
	{"selected_indexes", "9", ""},
	{"market", "10", ""},
	{"industry", "11", ""},
	{"legal_representative", "12", ""},
	{"registered_capital", "13", "float"},
	{"founded_date", "14", "date"},
	{"listing_date", "15", "date"},
	{"website", "16", ""},
	{"email", "17", ""},
	{"phone", "18", ""},
	{"fax", "19", ""},
	{"registered_address", "20", ""},
	{"office_address", "21", ""},
	{"postcode", "22", ""},
	{"main_business", "23", ""},
	{"business_scope", "24", ""},
	{"organization_profile", "25", ""},
}

// WEBAPI_STOCK_ALLOTMENT_FIELDS
var WEBAPI_STOCK_ALLOTMENT_FIELDS = []webapiField{
	{"record_id", "0", ""},
	{"name", "1", ""},
	{"suspend_start_date", "2", "date"},
	{"listing_announcement_date", "3", "date"},
	{"payment_start_date", "4", "date"},
	{"convertible_allotment_shares", "5", "float"},
	{"suspend_end_date", "6", "date"},
	{"actual_allotment_shares", "7", "float"},
	{"allotment_price", "8", "float"},
	{"allotment_ratio", "9", "float"},
	{"pre_total_share", "10", "float"},
	{"transfer_fee_per_share", "11", "float"},
	{"legal_person_actual_shares", "12", "float"},
	{"raised_funds_net", "13", "float"},
	{"major_shareholder_subscribe_method", "14", ""},
	{"other_allotment_name", "15", ""},
	{"issue_method", "16", ""},
	{"failed_refund_date", "17", "date"},
	{"ex_right_date", "18", "date"},
	{"expected_issue_expense", "19", "float"},
	{"issue_result_announcement_date", "20", "date"},
	{"warrant_trade_end_date", "22", "date"},
	{"other_actual_shares", "23", "float"},
	{"state_actual_shares", "24", "float"},
	{"entrusted_unit", "25", ""},
	{"public_transfer_shares", "26", "float"},
	{"other_allotment_code", "27", ""},
	{"allotment_target", "28", ""},
	{"warrant_trade_start_date", "29", "date"},
	{"fund_arrival_date", "30", "date"},
	{"organization_name", "31", ""},
	{"record_date", "32", "date"},
	{"raised_funds_gross", "33", "float"},
	{"expected_raised_funds", "34", "float"},
	{"major_shareholder_subscribe_shares", "35", "float"},
	{"public_actual_shares", "36", "float"},
	{"transfer_actual_shares", "37", "float"},
	{"underwriting_fee", "38", "float"},
	{"legal_person_transfer_shares", "39", "float"},
	{"post_float_share", "40", "float"},
	{"stock_class", "41", ""},
	{"public_allotment_name", "42", ""},
	{"issue_method_code", "43", ""},
	{"underwriting_method", "44", ""},
	{"announcement_date", "45", "date"},
	{"allotment_listing_date", "46", "date"},
	{"payment_end_date", "47", "date"},
	{"underwriting_balance", "48", "float"},
	{"expected_allotment_shares", "49", "float"},
	{"post_total_share", "50", "float"},
	{"employee_actual_shares", "51", "float"},
	{"underwriting_method_code", "52", ""},
	{"issue_expenses_total", "53", "float"},
	{"pre_float_share", "54", "float"},
	{"stock_class_code", "55", ""},
	{"public_allotment_code", "56", ""},
}

// WEBAPI_STOCK_DIVIDEND_FIELDS
var WEBAPI_STOCK_DIVIDEND_FIELDS = []webapiField{
	{"announcement_date", "F006D", "date"},
	{"dividend_type", "F044V", ""},
	{"bonus_share_ratio", "F011N", "float"},
	{"transfer_share_ratio", "F010N", "float"},
	{"cash_dividend_ratio", "F012N", "float"},
	{"record_date", "F018D", "date"},
	{"ex_right_date", "F020D", "date"},
	{"dividend_payment_date", "F023D", "date"},
	{"share_arrival_date", "F025D", "date"},
	{"plan_description", "F007V", ""},
	{"report_period", "F001V", ""},
}

// WEBAPI_STOCK_HOLD_CHANGE_FIELDS
var WEBAPI_STOCK_HOLD_CHANGE_FIELDS = []webapiField{
	{"circulated_share", "0", "float"},
	{"total_share", "1", "float"},
	{"trade_market", "2", ""},
	{"name", "3", ""},
	{"announcement_date", "4", "date"},
	{"change_reason", "5", ""},
	{"symbol", "6", ""},
	{"change_date", "7", "date"},
	{"restricted_share", "8", "float"},
	{"circulated_ratio", "9", "float"},
}

// WEBAPI_STOCK_HOLD_CONTROL_FIELDS
var WEBAPI_STOCK_HOLD_CONTROL_FIELDS = []webapiField{
	{"holding_ratio", "0", "float"},
	{"holding_shares", "1", "float"},
	{"name", "2", ""},
	{"actual_controller_name", "3", ""},
	{"direct_controller_name", "4", ""},
	{"control_type", "5", ""},
	{"symbol", "6", ""},
	{"change_date", "7", "date"},
}

// WEBAPI_STOCK_HOLD_NUM_FIELDS
var WEBAPI_STOCK_HOLD_NUM_FIELDS = []webapiField{
	{"avg_holding", "0", "float"},
	{"shareholder_count_change_pct", "1", "float"},
	{"prev_shareholder_count", "2", "float"},
	{"shareholder_count", "3", "float"},
	{"name", "4", ""},
	{"symbol", "5", ""},
	{"avg_holding_change_pct", "6", "float"},
	{"change_date", "7", "date"},
	{"prev_avg_holding", "8", "float"},
}

// WEBAPI_STOCK_INDUSTRY_CATEGORY_FIELDS
var WEBAPI_STOCK_INDUSTRY_CATEGORY_FIELDS = []webapiField{
	{"parent_code", "PARENTCODE", ""},
	{"category_code", "SORTCODE", ""},
	{"category_name", "SORTNAME", ""},
	{"category_name_en", "F001V", ""},
	{"end_date", "F002D", "date"},
	{"industry_type_code", "F003V", ""},
	{"industry_type", "F004V", ""},
}

// WEBAPI_STOCK_IPO_SUMMARY_FIELDS
var WEBAPI_STOCK_IPO_SUMMARY_FIELDS = []webapiField{
	{"source_symbol", "0", ""},
	{"prospectus_announcement_date", "1", "date"},
	{"lottery_rate_announcement_date", "2", "date"},
	{"par_value", "3", "float"},
	{"total_issue_shares", "4", "float"},
	{"nav_per_share_before_issue", "5", "float"},
	{"diluted_pe", "6", "float"},
	{"raised_funds_net", "7", "float"},
	{"online_issue_date", "8", "date"},
	{"listing_date", "9", "date"},
	{"issue_price", "10", "float"},
	{"issue_expenses_total", "11", "float"},
	{"nav_per_share_after_issue", "12", "float"},
	{"online_lottery_rate", "13", "float"},
	{"lead_underwriter", "14", ""},
}

// WEBAPI_STOCK_NEW_IPO_FIELDS
var WEBAPI_STOCK_NEW_IPO_FIELDS = []webapiField{
	{"lottery_result_announcement_date", "0", "date"},
	{"winning_announcement_date", "1", "date"},
	{"name", "2", ""},
	{"listing_date", "3", "date"},
	{"payment_date", "4", "date"},
	{"subscription_date", "5", "date"},
	{"issue_price", "6", "float"},
	{"symbol", "7", ""},
	{"online_lottery_rate", "8", "float"},
	{"total_issue_shares", "9", "float"},
	{"issue_pe", "10", "float"},
	{"online_issue_shares", "11", "float"},
	{"online_subscription_limit", "12", "float"},
}

// WEBAPI_STOCK_NEW_GH_FIELDS
var WEBAPI_STOCK_NEW_GH_FIELDS = []webapiField{
	{"company_name", "0", ""},
	{"meeting_date", "1", "date"},
	{"review_type", "2", ""},
	{"review_content", "3", ""},
	{"review_result", "4", ""},
	{"announcement_date", "5", "date"},
}

// WEBAPI_STOCK_SHARE_CHANGE_FIELDS
var WEBAPI_STOCK_SHARE_CHANGE_FIELDS = []webapiField{
	{"symbol", "SECCODE", ""},
	{"name", "SECNAME", ""},
	{"organization_name", "ORGNAME", ""},
	{"announcement_date", "DECLAREDATE", "date"},
	{"change_date", "VARYDATE", "date"},
	{"change_reason_code", "F001V", ""},
	{"change_reason", "F002V", ""},
	{"total_share", "F003N", "float"},
	{"non_circulating_share", "F004N", "float"},
	{"promoter_share", "F005N", "float"},
	{"state_share", "F006N", "float"},
	{"state_owned_legal_person_share", "F007N", "float"},
	{"domestic_legal_person_share", "F008N", "float"},
	{"foreign_legal_person_share", "F009N", "float"},
	{"natural_person_share", "F010N", "float"},
	{"raised_legal_person_share", "F011N", "float"},
	{"employee_share", "F012N", "float"},
	{"transferred_share", "F013N", "float"},
	{"other_restricted_share", "F014N", "float"},
	{"preferred_share", "F015N", "float"},
	{"other_non_circulating_share", "F016N", "float"},
	{"circulating_share", "F021N", "float"},
	{"a_share", "F022N", "float"},
	{"b_share", "F023N", "float"},
	{"h_share", "F024N", "float"},
	{"executive_share", "F025N", "float"},
	{"other_circulating_share", "F026N", "float"},
	{"restricted_share", "F028N", "float"},
	{"allocated_legal_person_share", "F017N", "float"},
	{"strategic_investor_share", "F018N", "float"},
	{"securities_investment_fund_share", "F019N", "float"},
	{"general_legal_person_share", "F020N", "float"},
	{"state_restricted_share", "F029N", "float"},
	{"state_owned_legal_person_restricted_share", "F030N", "float"},
	{"other_domestic_restricted_share", "F031N", "float"},
	{"domestic_legal_person_restricted_share", "F032N", "float"},
	{"domestic_natural_person_restricted_share", "F033N", "float"},
	{"foreign_restricted_share", "F034N", "float"},
	{"foreign_legal_person_restricted_share", "F035N", "float"},
	{"foreign_natural_person_restricted_share", "F036N", "float"},
	{"restricted_executive_share", "F037N", "float"},
	{"restricted_b_share", "F038N", "float"},
	{"restricted_h_share", "F040N", "float"},
	{"controlling_shareholder_actual_controller_share", "F050N", "float"},
}

// WEBAPI_FUND_ASSET_ALLOCATION_FIELDS
var WEBAPI_FUND_ASSET_ALLOCATION_FIELDS = []webapiField{
	{"report_date", "ENDDATE", "date"},
	{"fund_count", "F001N", "float"},
	{"equity_asset_pct", "F006N", "float"},
	{"bond_asset_pct", "F007N", "float"},
	{"cash_asset_pct", "F008N", "float"},
	{"fund_market_net_assets", "F005N", "float"},
}

// WEBAPI_FUND_INDUSTRY_ALLOCATION_FIELDS
var WEBAPI_FUND_INDUSTRY_ALLOCATION_FIELDS = []webapiField{
	{"industry_code", "F001V", ""},
	{"industry_name", "F002V", ""},
	{"report_date", "ENDDATE", "date"},
	{"fund_count", "F003N", "float"},
	{"industry_scale", "F004N", "float"},
	{"net_asset_pct", "F005N", "float"},
}

// WEBAPI_FUND_STOCK_FIELDS
var WEBAPI_FUND_STOCK_FIELDS = []webapiField{
	{"record_id", "ID", ""},
	{"symbol", "SECCODE", ""},
	{"name", "SECNAME", ""},
	{"report_date", "ENDDATE", "date"},
	{"fund_count", "F001N", "float"},
	{"holding_shares", "F002N", "float"},
	{"holding_market_value", "F003N", "float"},
}

// WEBAPI_STOCK_CG_EQUITY_MORTGAGE_FIELDS
var WEBAPI_STOCK_CG_EQUITY_MORTGAGE_FIELDS = []webapiField{
	{"released_pledge_shares", "0", "float"},
	{"name", "1", ""},
	{"announcement_date", "2", "date"},
	{"pledge_event", "3", ""},
	{"pledgee", "4", ""},
	{"pledgor", "5", ""},
	{"symbol", "6", ""},
	{"pledged_total_share_pct", "7", "float"},
	{"cumulative_pledge_total_share_pct", "8", "float"},
	{"pledged_shares", "9", "float"},
}

// WEBAPI_STOCK_CG_GUARANTEE_FIELDS
var WEBAPI_STOCK_CG_GUARANTEE_FIELDS = []webapiField{
	{"announcement_period", "0", ""},
	{"guarantee_amount_net_asset_pct", "1", "float"},
	{"guarantee_amount", "2", "float"},
	{"guarantee_count", "3", "float"},
	{"name", "4", ""},
	{"symbol", "5", ""},
	{"parent_equity", "6", "float"},
}

// WEBAPI_STOCK_CG_LAWSUIT_FIELDS
var WEBAPI_STOCK_CG_LAWSUIT_FIELDS = []webapiField{
	{"announcement_period", "0", ""},
	{"lawsuit_amount", "1", "float"},
	{"lawsuit_count", "2", "float"},
	{"name", "3", ""},
	{"symbol", "4", ""},
}

// WEBAPI_STOCK_HOLD_MANAGEMENT_DETAIL_FIELDS
var WEBAPI_STOCK_HOLD_MANAGEMENT_DETAIL_FIELDS = []webapiField{
	{"name", "0", ""},
	{"announcement_date", "1", "date"},
	{"executive_name", "2", ""},
	{"ending_market_value", "3", "float"},
	{"average_price", "4", "float"},
	{"symbol", "5", ""},
	{"change_ratio", "6", "float"},
	{"change_shares", "7", "float"},
	{"end_date", "8", "date"},
	{"ending_holding_shares", "9", "float"},
	{"beginning_holding_shares", "10", "float"},
	{"changer_relation", "11", ""},
	{"director_supervisor_senior_position", "12", ""},
	{"director_supervisor_senior_name", "13", ""},
	{"data_source", "14", ""},
	{"change_reason", "15", ""},
}

// WEBAPI_STOCK_INDUSTRY_CHANGE_FIELDS
var WEBAPI_STOCK_INDUSTRY_CHANGE_FIELDS = []webapiField{
	{"organization_name", "ORGNAME", ""},
	{"symbol", "SECCODE", ""},
	{"name", "SECNAME", ""},
	{"change_date", "VARYDATE", "date"},
	{"classification_standard_code", "F001V", ""},
	{"classification_standard", "F002V", ""},
	{"industry_code", "F003V", ""},
	{"industry_sector", "F004V", ""},
	{"industry_subcategory", "F005V", ""},
	{"industry_major", "F006V", ""},
	{"industry_middle", "F007V", ""},
	{"latest_record_flag", "F008C", ""},
}

// WEBAPI_STOCK_INDUSTRY_PE_RATIO_FIELDS
var WEBAPI_STOCK_INDUSTRY_PE_RATIO_FIELDS = []webapiField{
	{"industry_level", "0", "int"},
	{"static_pe_mean", "1", "float"},
	{"static_pe_median", "2", "float"},
	{"static_pe_weighted", "3", "float"},
	{"net_profit_static", "4", "float"},
	{"industry_name", "5", ""},
	{"industry_code", "6", ""},
	{"classification", "7", ""},
	{"total_market_value_static", "8", "float"},
	{"included_company_count", "9", "float"},
	{"change_date", "10", "date"},
	{"company_count", "11", "float"},
}

// WEBAPI_STOCK_RANK_FORECAST_FIELDS
var WEBAPI_STOCK_RANK_FORECAST_FIELDS = []webapiField{
	{"name", "0", ""},
	{"publish_date", "1", "date"},
	{"previous_rating", "2", ""},
	{"rating_change", "3", ""},
	{"target_price_high", "4", "float"},
	{"is_first_rating", "5", ""},
	{"rating", "6", ""},
	{"analyst_name", "7", ""},
	{"institution_short_name", "8", ""},
	{"target_price_low", "9", "float"},
	{"symbol", "10", ""},
}

// WEBAPI_BOND_CORPORATE_ISSUE_FIELDS
var WEBAPI_BOND_CORPORATE_ISSUE_FIELDS = []webapiField{
	{"bond_code", "SECCODE", ""},
	{"bond_short_name", "SECNAME", ""},
	{"announcement_date", "DECLAREDATE", "date"},
	{"online_issue_start_date", "F003D", "date"},
	{"online_issue_end_date", "F004D", "date"},
	{"planned_issue_amount", "F005N", "float"},
	{"actual_issue_amount", "F006N", "float"},
	{"par_value", "F008N", "float"},
	{"issue_price", "F007N", "float"},
	{"issue_method", "F013V", ""},
	{"issue_target", "F014V", ""},
	{"issue_scope", "F015V", ""},
	{"underwriting_method", "F017V", ""},
	{"min_subscription_unit", "F022N", "float"},
	{"fundraising_use", "F023V", ""},
	{"min_subscription_amount", "F052N", "float"},
	{"bond_name", "BONDNAME", ""},
}

// WEBAPI_BOND_COV_ISSUE_FIELDS
var WEBAPI_BOND_COV_ISSUE_FIELDS = []webapiField{
	{"bond_code", "SECCODE", ""},
	{"bond_short_name", "SECNAME", ""},
	{"announcement_date", "DECLAREDATE", "date"},
	{"issue_start_date", "F029D", "date"},
	{"issue_end_date", "F003D", "date"},
	{"planned_issue_amount", "F005N", "float"},
	{"actual_issue_amount", "F006N", "float"},
	{"par_value", "F007N", "float"},
	{"issue_price", "F052N", "float"},
	{"issue_method", "F013V", ""},
	{"issue_target", "F014V", ""},
	{"issue_scope", "F015V", ""},
	{"underwriting_method", "F017V", ""},
	{"fundraising_use", "F021V", ""},
	{"initial_conversion_price", "F026N", "float"},
	{"conversion_start_date", "F027D", "date"},
	{"conversion_end_date", "F053D", "date"},
	{"online_subscription_date", "F051D", "date"},
	{"online_subscription_code", "F031V", ""},
	{"online_subscription_short_name", "F032V", ""},
	{"online_subscription_max", "F008N", "float"},
	{"online_subscription_min", "F066N", "float"},
	{"online_subscription_unit", "F067N", "float"},
	{"online_lottery_result_refund_date", "F068D", "date"},
	{"priority_subscription_date", "F004D", "date"},
	{"allotment_price", "F065N", "float"},
	{"bondholder_record_date", "F028D", "date"},
	{"priority_subscription_payment_date", "F054D", "date"},
	{"conversion_code", "F086V", ""},
	{"trading_market", "F002V", ""},
	{"bond_name", "BONDNAME", ""},
}

// WEBAPI_BOND_COV_STOCK_ISSUE_FIELDS
var WEBAPI_BOND_COV_STOCK_ISSUE_FIELDS = []webapiField{
	{"bond_code", "SECCODE", ""},
	{"bond_short_name", "SECNAME", ""},
	{"announcement_date", "DECLAREDATE", "date"},
	{"conversion_code", "F001V", ""},
	{"conversion_short_name", "F002V", ""},
	{"conversion_price", "F003N", "float"},
	{"voluntary_conversion_start_date", "F004D", "date"},
	{"voluntary_conversion_end_date", "F005D", "date"},
	{"underlying_stock", "F017V", ""},
	{"bond_name", "BONDNAME", ""},
}

// WEBAPI_BOND_PUBLIC_ISSUE_FIELDS
var WEBAPI_BOND_PUBLIC_ISSUE_FIELDS = []webapiField{
	{"bond_code", "SECCODE", ""},
	{"bond_short_name", "SECNAME", ""},
	{"issue_start_date", "F004D", "date"},
	{"issue_end_date", "F003D", "date"},
	{"planned_issue_amount", "F006N", "float"},
	{"actual_issue_amount", "F005N", "float"},
	{"issue_price", "F007N", "float"},
	{"par_value", "F008N", "float"},
	{"payment_date", "F009D", "date"},
	{"additional_issue_count", "F028N", "float"},
	{"trading_market", "F002V", ""},
	{"issue_method", "F013V", ""},
	{"issue_target", "F014V", ""},
	{"announcement_date", "DECLAREDATE", "date"},
	{"bond_name", "BONDNAME", ""},
}
