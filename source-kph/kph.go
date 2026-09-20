package kph

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	KPH_REALTIME_URL = "https://apphwshhq.kaipanhong.com/w1/api/index.php"
	KPH_HISTORY_URL  = "https://apphis.kaipanhong.com/w1/api/index.php"
)

var _BASE_PARAMS = url.Values{
	"PhoneOSNew": {"1"},
	"DeviceID":   {"1a609dd6-b2b8-3bf9-ac40-a77581551454"},
	"VerSion":    {"6.0.6"},
	"Token":      {"0"},
	"UserID":     {"0"},
	"Red":        {"1"},
	"apiv":       {"w45"},
}

var (
	sectorTypeMap = map[string]string{
		"selected": "7", "industry": "4", "region": "6",
		"7": "7", "4": "4", "6": "6",
	}
	limitPID = map[string]string{
		"up": "4", "down": "3", "wind_vane": "6",
	}
)

var (
	digitOnlyRe = regexp.MustCompile(`\D`)
)

// Adapter interface
type Adapter interface {
	Name() string
	Description() string
	Request(ctx context.Context, params map[string]interface{}) ([]map[string]interface{}, error)
}

// KPHAdapter implements the KPH (开盘红) API.
type KPHAdapter struct {
	client      *http.Client
	description string
}

// NewKPHAdapter creates a new KPH adapter.
func NewKPHAdapter() *KPHAdapter {
	return &KPHAdapter{
		client:      &http.Client{Timeout: 30 * time.Second},
		description: "KPH (开盘红) API adapter",
	}
}

// Name returns the adapter name.
func (a *KPHAdapter) Name() string {
	return "kph"
}

// Description returns adapter info.
func (a *KPHAdapter) Description() string {
	return a.description
}

// Request fetches data from KPH.
func (a *KPHAdapter) Request(ctx context.Context, params map[string]interface{}) ([]map[string]interface{}, error) {
	interfaceName := ""
	if v, ok := params["interface"]; ok {
		interfaceName = valueToString(v)
	}
	if interfaceName == "" {
		return nil, fmt.Errorf("interface parameter is required")
	}

	switch interfaceName {
	case "kph_sector_plate_detail":
		return a.requestSectorPlateDetail(ctx, params)
	case "kph_sector_concept_detail":
		return a.requestSectorConceptDetail(ctx, params)
	case "kph_market_emotion":
		return a.requestMarketEmotion(ctx, params)
	case "kph_sector_ranking":
		return a.requestSectorRanking(ctx, params)
	case "kph_sector_constituents_history":
		return a.requestSectorConstituentsHistory(ctx, params)
	case "kph_limit_up_history":
		return a.requestLimitHistory(ctx, params, "up")
	case "kph_limit_down_history":
		return a.requestLimitHistory(ctx, params, "down")
	case "kph_wind_vane_history":
		return a.requestLimitHistory(ctx, params, "wind_vane")
	case "kph_limit_ladder":
		return a.requestLimitLadder(ctx, params)
	case "kph_market_review_events":
		return a.requestMarketReviewEvents(ctx, params)
	case "kph_limit_resumption_history":
		return a.requestLimitResumptionHistory(ctx, params)
	default:
		return nil, fmt.Errorf("KPH source adapter does not support interface %q", interfaceName)
	}
}

func (a *KPHAdapter) requestSectorPlateDetail(ctx context.Context, params map[string]interface{}) ([]map[string]interface{}, error) {
	plateID, err := requiredText(params, "stock_code")
	if err != nil {
		return nil, err
	}
	tradeDate, err := normalizeDate(params, false)
	if err != nil {
		return nil, err
	}
	if tradeDate == "" {
		tradeDate = todayDash()
	}
	host := KPH_REALTIME_URL
	if tradeDate != todayDash() {
		host = KPH_HISTORY_URL
	}

	p := map[string]string{
		"a":          "plate_detail",
		"c":          "ZhiShuRanking",
		"stock_code": plateID,
		"Date":       tradeDate,
	}
	payload, err := a.post(ctx, params, host, p, "KPH sector plate detail")
	if err != nil {
		return nil, err
	}

	var rows []map[string]interface{}
	for _, row := range payloadList(payload) {
		r := parsePlateDetailRow(row, tradeDate, plateID)
		rows = append(rows, r)
	}
	return rows, nil
}

func parsePlateDetailRow(row []interface{}, tradeDate, plateID string) map[string]interface{} {
	identity := identityFromSymbol(at(row, 0))
	d8 := strings.ReplaceAll(tradeDate, "-", "")
	return map[string]interface{}{
		"trade_date":    d8,
		"plate_id":      plateID,
		"instrument_id": identity["instrument_id"],
		"symbol":        identity["symbol"],
		"exchange":      identity["exchange"],
		"name":          cleanText(at(row, 1)),
		"last_price":    parseFloat(at(row, 5)),
		"change_pct":    parseFloat(at(row, 6)),
		"amount":        parseFloat(at(row, 7)),
		"turnover_rate": parseFloat(at(row, 8)),
		"float_mv":      parseFloat(at(row, 10)),
		"net_inflow":    parseFloat(at(row, 12)),
		"main_net":      parseFloat(at(row, 13)),
	}
}

func (a *KPHAdapter) requestSectorConceptDetail(ctx context.Context, params map[string]interface{}) ([]map[string]interface{}, error) {
	plateID, err := requiredText(params, "stock_code")
	if err != nil {
		return nil, err
	}
	tradeDate, err := normalizeDate(params, false)
	if err != nil {
		return nil, err
	}
	if tradeDate == "" {
		tradeDate = todayDash()
	}
	host := KPH_REALTIME_URL
	if tradeDate != todayDash() {
		host = KPH_HISTORY_URL
	}

	p := map[string]string{
		"a":          "concept_detail",
		"c":          "ZhiShuRanking",
		"stock_code": plateID,
		"Date":       tradeDate,
	}
	payload, err := a.post(ctx, params, host, p, "KPH sector concept detail")
	if err != nil {
		return nil, err
	}

	var rows []map[string]interface{}
	for _, row := range payloadList(payload) {
		r := parsePlateDetailRow(row, tradeDate, plateID)
		rows = append(rows, r)
	}
	return rows, nil
}

func (a *KPHAdapter) requestMarketEmotion(ctx context.Context, params map[string]interface{}) ([]map[string]interface{}, error) {
	tradeDate, err := normalizeDate(params, false)
	if err != nil {
		return nil, err
	}
	if tradeDate == "" {
		tradeDate = todayDash()
	}
	isToday := tradeDate == todayDash()
	host := KPH_REALTIME_URL
	if !isToday {
		host = KPH_HISTORY_URL
	}

	p := map[string]string{
		"a": "ZhangFuDetail",
		"c": "HomeDingPan",
	}
	if !isToday {
		p["a"] = "HisZhangFuDetail"
		p["c"] = "HisHomeDingPan"
		p["Day"] = tradeDate
	}

	payload, err := a.post(ctx, params, host, p, "KPH market emotion")
	if err != nil {
		return nil, err
	}

	info, _ := payload["info"].(map[string]interface{})
	if info == nil {
		info = map[string]interface{}{}
	}

	d8 := strings.ReplaceAll(tradeDate, "-", "")
	row := map[string]interface{}{
		"trade_date":            d8,
		"limit_up_count":        parseInt(info["ZT"]),
		"limit_down_count":      parseInt(info["DT"]),
		"real_limit_up_count":   parseInt(info["SJZT"]),
		"real_limit_down_count": parseInt(info["SJDT"]),
		"st_limit_up_count":     parseInt(info["STZT"]),
		"st_limit_down_count":   parseInt(info["STDT"]),
		"rise_count":            parseInt(info["SZJS"]),
		"fall_count":            parseInt(info["XDJS"]),
		"flat_count":            parseInt(info["0"]),
		"market_sentiment":      cleanText(info["sign"]),
		"market_amount":         parseFloat(info["qscln"]),
		"raw_rise_dist":         buildRiseDist(info),
		"raw_fall_dist":         buildFallDist(info),
	}
	return []map[string]interface{}{row}, nil
}

func buildRiseDist(info map[string]interface{}) map[string]interface{} {
	m := map[string]interface{}{}
	for i := 1; i <= 10; i++ {
		m[strconv.Itoa(i)] = parseInt(info[strconv.Itoa(i)])
	}
	return m
}

func buildFallDist(info map[string]interface{}) map[string]interface{} {
	m := map[string]interface{}{}
	for i := -1; i >= -10; i-- {
		m[strconv.Itoa(i)] = parseInt(info[strconv.Itoa(i)])
	}
	return m
}

func (a *KPHAdapter) requestSectorRanking(ctx context.Context, params map[string]interface{}) ([]map[string]interface{}, error) {
	tradeDate, err := normalizeDate(params, true)
	if err != nil {
		return nil, err
	}
	sectorType := ""
	if v, ok := params["sector_type"]; ok {
		sectorType = strings.ToLower(strings.TrimSpace(valueToString(v)))
	}
	if sectorType == "" {
		sectorType = "selected"
	}
	zsType, ok := sectorTypeMap[sectorType]
	if !ok {
		return nil, fmt.Errorf("sector_type must be selected, industry, or region")
	}

	fetchAll := false
	if v, ok := params["fetch_all"]; ok {
		fetchAll = parseBoolParam(v, false)
	}

	host := KPH_REALTIME_URL
	if tradeDate != todayDash() {
		host = KPH_HISTORY_URL
	}
	typeParam := "1"
	if zsType != "7" {
		typeParam = "2"
	}

	pageSize := 50
	index := 0
	var rows []map[string]interface{}
	for {
		p := map[string]string{
			"a":      "RealRankingInfo",
			"c":      "ZhiShuRanking",
			"Order":  "1",
			"st":     strconv.Itoa(pageSize),
			"Index":  strconv.Itoa(index),
			"Date":   tradeDate,
			"Type":   typeParam,
			"ZSType": zsType,
		}
		payload, err := a.post(ctx, params, host, p, "KPH sector ranking")
		if err != nil {
			return nil, err
		}
		batch := payloadList(payload)
		for _, row := range batch {
			r := parseSectorRow(row, tradeDate, sectorType)
			rows = append(rows, r)
		}
		if !fetchAll || len(batch) < pageSize {
			break
		}
		index += pageSize
	}
	return rows, nil
}

func parseSectorRow(row []interface{}, tradeDate, sectorType string) map[string]interface{} {
	d8 := strings.ReplaceAll(tradeDate, "-", "")
	return map[string]interface{}{
		"trade_date":    d8,
		"plate_id":      cleanText(at(row, 0)),
		"plate_name":    cleanText(at(row, 1)),
		"sector_type":   sectorType,
		"amount":        parseFloat(at(row, 2)),
		"change_pct":    parseFloat(at(row, 3)),
		"amplitude":     parseFloat(at(row, 4)),
		"net_inflow":    parseFloat(at(row, 5)),
		"turnover_rate": parseFloat(at(row, 9)),
		"market_cap":    parseFloat(at(row, 10)),
		"stock_count":   parseInt(at(row, 17)),
	}
}

func (a *KPHAdapter) requestSectorConstituentsHistory(ctx context.Context, params map[string]interface{}) ([]map[string]interface{}, error) {
	plateID, err := requiredText(params, "plate_id")
	if err != nil {
		return nil, err
	}
	tradeDate, err := historicalDate(params)
	if err != nil {
		return nil, err
	}

	p := map[string]string{
		"a":          "ZhiShuStockList_W8",
		"c":          "ZhiShuRanking",
		"Order":      "1",
		"st":         "1000",
		"old":        "1",
		"Index":      "0",
		"Date":       tradeDate,
		"Type":       "6",
		"PlateID":    plateID,
		"IsZZ":       "0",
		"IsKZZType":  "0",
		"TSZB":       "0",
		"TSZB_Type":  "0",
		"filterType": "0",
	}
	payload, err := a.post(ctx, params, KPH_HISTORY_URL, p, "KPH sector constituents history")
	if err != nil {
		return nil, err
	}

	var rows []map[string]interface{}
	for _, row := range payloadList(payload) {
		r := parseSectorConstituentRow(row, tradeDate, plateID)
		rows = append(rows, r)
	}
	return rows, nil
}

func parseSectorConstituentRow(row []interface{}, tradeDate, plateID string) map[string]interface{} {
	identity := identityFromSymbol(at(row, 0))
	d8 := strings.ReplaceAll(tradeDate, "-", "")
	m := map[string]interface{}{
		"trade_date":         d8,
		"plate_id":           plateID,
		"instrument_id":      identity["instrument_id"],
		"symbol":             identity["symbol"],
		"exchange":           identity["exchange"],
		"name":               cleanText(at(row, 1)),
		"tags":               cleanText(at(row, 4)),
		"last_price":         parseFloat(at(row, 5)),
		"change_pct":         parseFloat(at(row, 6)),
		"amount":             parseFloat(at(row, 7)),
		"turnover_rate":      parseFloat(at(row, 8)),
		"float_market_value": parseFloat(at(row, 10)),
		"main_net":           parseFloat(at(row, 13)),
		"limit_tag":          cleanText(at(row, 23)),
		"rank_tag":           cleanText(at(row, 24)),
		"limit_count":        parseInt(at(row, 40)),
	}
	return m
}

func (a *KPHAdapter) requestLimitHistory(ctx context.Context, params map[string]interface{}, kind string) ([]map[string]interface{}, error) {
	tradeDate, err := historicalDate(params)
	if err != nil {
		return nil, err
	}
	pid, ok := limitPID[kind]
	if !ok {
		return nil, fmt.Errorf("unknown limit kind: %s", kind)
	}

	p := map[string]string{
		"a":                 "HisDaBanList",
		"c":                 "HisHomeDingPan",
		"Order":             "1",
		"st":                "50",
		"Index":             "0",
		"Is_st":             "1",
		"PidType":           pid,
		"Type":              "6",
		"FilterMotherboard": "0",
		"Filter":            "0",
		"FilterTIB":         "0",
		"FilterGem":         "0",
		"Day":               tradeDate,
	}
	payload, err := a.post(ctx, params, KPH_HISTORY_URL, p, fmt.Sprintf("KPH %s history", kind))
	if err != nil {
		return nil, err
	}

	var rows []map[string]interface{}
	for _, row := range payloadList(payload) {
		r := parseLimitHistoryRow(row, tradeDate, kind)
		rows = append(rows, r)
	}
	return rows, nil
}

func parseLimitHistoryRow(row []interface{}, tradeDate, kind string) map[string]interface{} {
	isUpLike := kind == "up" || kind == "wind_vane"
	identity := identityFromSymbol(at(row, 0))
	d8 := strings.ReplaceAll(tradeDate, "-", "")
	m := map[string]interface{}{
		"trade_date":    tradeDate,
		"instrument_id": identity["instrument_id"],
		"symbol":        identity["symbol"],
		"exchange":      identity["exchange"],
		"name":          cleanText(at(row, 1)),
		"limit_time":    parseInt(at(row, 6)),
		"open_time":     parseInt(at(row, 7)),
		"seal_amount":   parseFloat(at(row, 8)),
		"seal_money":    parseFloat(at(row, 23)),
		"turnover":      parseFloat(at(row, 13)),
		"turnover_rate": parseFloat(at(row, 14)),
		"market_cap":    parseFloat(at(row, 15)),
		"net_inflow":    parseFloat(at(row, 12)),
		"industry_id":   cleanText(at(row, 26)),
	}
	// Override trade_date to d8 format
	m["trade_date"] = d8

	if isUpLike {
		m["limit_tag"] = cleanText(at(row, 9))
		m["limit_count"] = parseInt(at(row, 10))
		m["themes"] = cleanText(at(row, 11))
		m["reason"] = cleanText(at(row, 16))
		m["industry_limit_up_count"] = parseInt(at(row, 27))
	} else {
		m["themes"] = cleanText(at(row, 11))
	}

	return m
}

func (a *KPHAdapter) requestLimitLadder(ctx context.Context, params map[string]interface{}) ([]map[string]interface{}, error) {
	tradeDate, err := normalizeDate(params, false)
	if err != nil {
		return nil, err
	}
	if tradeDate == "" {
		tradeDate = todayDash()
	}
	host := KPH_REALTIME_URL
	if tradeDate != todayDash() {
		host = KPH_HISTORY_URL
	}

	p := map[string]string{
		"a":    "GetZhangTingTianTi",
		"c":    "FuPanLa",
		"Date": tradeDate,
	}
	payload, err := a.post(ctx, params, host, p, "KPH limit ladder")
	if err != nil {
		return nil, err
	}

	stockList, _ := payload["StockList"].([]interface{})
	if stockList == nil {
		stockList, _ = payload["stock_list"].([]interface{})
	}

	var rows []map[string]interface{}
	if stockList == nil {
		return rows, nil
	}
	for _, group := range stockList {
		groupArr, ok := group.([]interface{})
		if !ok {
			continue
		}
		if len(groupArr) == 0 {
			continue
		}
		// Check if first element is a list (grouped ladder)
		if inner, ok := groupArr[0].([]interface{}); ok {
			for _, row := range inner {
				if rowArr, ok := row.([]interface{}); ok {
					r := parseLadderRow(rowArr, tradeDate)
					rows = append(rows, r)
				}
			}
		} else {
			r := parseLadderRow(groupArr, tradeDate)
			rows = append(rows, r)
		}
	}
	return rows, nil
}

func parseLadderRow(row []interface{}, tradeDate string) map[string]interface{} {
	identity := identityFromSymbol(at(row, 0))
	d8 := strings.ReplaceAll(tradeDate, "-", "")
	return map[string]interface{}{
		"trade_date":           d8,
		"instrument_id":        identity["instrument_id"],
		"symbol":               identity["symbol"],
		"exchange":             identity["exchange"],
		"name":                 cleanText(at(row, 1)),
		"limit_count":          parseInt(at(row, 2)),
		"limit_time":           parseInt(at(row, 3)),
		"plate_id":             cleanText(at(row, 4)),
		"plate_name":           cleanText(at(row, 5)),
		"one_word":             parseBoolValue(at(row, 6)),
		"popular":              parseBoolValue(at(row, 7)),
		"plate_limit_up_count": parseInt(at(row, 8)),
		"amount":               parseFloat(at(row, 9)),
		"plate_amount":         parseFloat(at(row, 10)),
	}
}

func (a *KPHAdapter) requestMarketReviewEvents(ctx context.Context, params map[string]interface{}) ([]map[string]interface{}, error) {
	tradeDate, err := normalizeDate(params, false)
	if err != nil {
		return nil, err
	}
	if tradeDate == "" {
		tradeDate = todayDash()
	}
	host := KPH_REALTIME_URL
	if tradeDate != todayDash() {
		host = KPH_HISTORY_URL
	}

	limit := 30
	if v, ok := params["limit"]; ok {
		if iv := paramAsInt(v, 0); iv > 0 {
			limit = iv
		}
	}
	offset := 0
	if v, ok := params["offset"]; ok {
		if iv := paramAsInt(v, -1); iv >= 0 {
			offset = iv
		}
	}

	p := map[string]string{
		"a":     "GetPMSL_PMLD",
		"c":     "FuPanLa",
		"st":    strconv.Itoa(limit),
		"Index": strconv.Itoa(offset),
		"Date":  tradeDate,
	}
	payload, err := a.post(ctx, params, host, p, "KPH market review events")
	if err != nil {
		return nil, err
	}

	var items []map[string]interface{}
	list, _ := payload["List"].([]interface{})
	if list == nil {
		list, _ = payload["list"].([]interface{})
	}
	if list == nil {
		return nil, nil
	}
	for _, item := range list {
		if m, ok := item.(map[string]interface{}); ok {
			r := parseMarketEvent(m, tradeDate)
			items = append(items, r)
		}
	}
	return items, nil
}

func parseMarketEvent(item map[string]interface{}, tradeDate string) map[string]interface{} {
	d8 := strings.ReplaceAll(tradeDate, "-", "")
	stockList := make([]interface{}, 0)
	if sl, ok := item["StockList"].([]interface{}); ok {
		stockList = sl
	}
	return map[string]interface{}{
		"trade_date":     d8,
		"event_time":     parseInt(item["TimeMin"]),
		"tag_id":         parseInt(item["TagID"]),
		"tag_name":       cleanText(item["TagName"]),
		"tag_attribute":  parseInt(item["TagShuXing"]),
		"plate_id":       cleanText(item["ZSCode"]),
		"plate_name":     cleanText(item["ZSName"]),
		"detail":         cleanText(item["Detail"]),
		"raw_stock_list": stockList,
	}
}

func (a *KPHAdapter) requestLimitResumptionHistory(ctx context.Context, params map[string]interface{}) ([]map[string]interface{}, error) {
	tradeDate, err := normalizeDate(params, false)
	if err != nil {
		return nil, err
	}
	if tradeDate == "" {
		tradeDate = todayDash()
	}

	limit := 100
	if v, ok := params["limit"]; ok {
		if iv, ok := v.(int); ok && iv > 0 {
			limit = iv
		}
	}
	offset := 0
	if v, ok := params["offset"]; ok {
		if iv, ok := v.(int); ok && iv >= 0 {
			offset = iv
		}
	}

	p := map[string]string{
		"a":     "GetPlateInfo_w38",
		"c":     "HisLimitResumption",
		"st":    strconv.Itoa(limit),
		"Index": strconv.Itoa(offset),
		"Date":  tradeDate,
	}
	payload, err := a.post(ctx, params, KPH_HISTORY_URL, p, "KPH limit resumption history")
	if err != nil {
		return nil, err
	}

	var rows []map[string]interface{}
	list, _ := payload["list"].([]interface{})
	if list == nil {
		return rows, nil
	}
	for _, plate := range list {
		plateMap, ok := plate.(map[string]interface{})
		if !ok {
			continue
		}
		stockList, _ := plateMap["StockList"].([]interface{})
		for _, row := range stockList {
			rowArr, ok := row.([]interface{})
			if !ok {
				continue
			}
			r := parseResumptionStock(rowArr, plateMap, tradeDate)
			rows = append(rows, r)
		}
	}
	return rows, nil
}

func parseResumptionStock(row []interface{}, plate map[string]interface{}, tradeDate string) map[string]interface{} {
	identity := identityFromSymbol(at(row, 0))
	d8 := strings.ReplaceAll(tradeDate, "-", "")
	return map[string]interface{}{
		"trade_date":    d8,
		"plate_id":      cleanText(plate["ZSCode"]),
		"plate_name":    cleanText(plate["ZSName"]),
		"instrument_id": identity["instrument_id"],
		"symbol":        identity["symbol"],
		"exchange":      identity["exchange"],
		"name":          cleanText(at(row, 1)),
		"limit_tag":     cleanText(at(row, 9)),
		"limit_count":   parseInt(at(row, 10)),
		"themes":        cleanText(at(row, 11)),
		"reason_short":  cleanText(at(row, 16)),
		"reason_detail": cleanText(at(row, 17)),
	}
}

func (a *KPHAdapter) post(ctx context.Context, params map[string]interface{}, host string, extraParams map[string]string, context string) (map[string]interface{}, error) {
	all := make(url.Values)
	for k, v := range _BASE_PARAMS {
		all[k] = v
	}
	for k, v := range extraParams {
		all[k] = []string{v}
	}

	body := all.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, host, strings.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("%s request failed: %w", context, err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=UTF-8")
	req.Header.Set("User-Agent", "Dalvik/2.1.0 AxData/0.1")
	req.Header.Set("Accept", "application/json,text/plain,*/*")

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s request failed: %w", context, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s HTTP %d", context, resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%s request failed: %w", context, err)
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, fmt.Errorf("%s returned invalid JSON", context)
	}

	if errcode, ok := payload["errcode"]; ok && errcode != nil {
		s := valueToString(errcode)
		if s != "" && s != "0" {
			return nil, fmt.Errorf("%s returned errcode=%v", context, errcode)
		}
	}
	return payload, nil
}

func payloadList(payload map[string]interface{}) [][]interface{} {
	list, _ := payload["list"].([]interface{})
	var rows [][]interface{}
	for _, item := range list {
		if arr, ok := item.([]interface{}); ok {
			rows = append(rows, arr)
		}
	}
	return rows
}

func normalizeDate(params map[string]interface{}, required bool) (string, error) {
	v := params["trade_date"]
	if v == nil || valueToString(v) == "" {
		if required {
			return "", fmt.Errorf("trade_date is required")
		}
		return "", nil
	}
	text := valueToString(v)
	digits := digitOnlyRe.ReplaceAllString(text, "")
	if len(digits) != 8 {
		return "", fmt.Errorf("trade_date must be YYYYMMDD or YYYY-MM-DD")
	}
	_, err := time.ParseInLocation("20060102", digits, time.UTC)
	if err != nil {
		return "", fmt.Errorf("trade_date must be a valid date")
	}
	return fmt.Sprintf("%s-%s-%s", digits[:4], digits[4:6], digits[6:8]), nil
}

func historicalDate(params map[string]interface{}) (string, error) {
	tradeDate, err := normalizeDate(params, true)
	if err != nil {
		return "", err
	}
	date := strings.ReplaceAll(tradeDate, "-", "")
	_, err = time.ParseInLocation("20060102", date, time.UTC)
	if err != nil {
		return "", fmt.Errorf("trade_date must be a valid date")
	}
	if tradeDate >= todayDash() {
		return "", fmt.Errorf("trade_date must be earlier than today for this KPH historical interface")
	}
	return tradeDate, nil
}

func todayDash() string {
	return time.Now().Format("2006-01-02")
}

func identityFromSymbol(v interface{}) map[string]interface{} {
	if v == nil {
		return map[string]interface{}{
			"instrument_id": nil,
			"symbol":        nil,
			"exchange":      nil,
		}
	}
	symbol := valueToString(v)
	clean := digitOnlyRe.ReplaceAllString(symbol, "")
	if len(clean) != 6 {
		return map[string]interface{}{
			"instrument_id": nil,
			"symbol":        nil,
			"exchange":      nil,
		}
	}
	var exchange, suffix string
	if strings.HasPrefix(clean, "6") || strings.HasPrefix(clean, "5") || strings.HasPrefix(clean, "9") {
		exchange = "SSE"
		suffix = "SH"
	} else if strings.HasPrefix(clean, "4") || strings.HasPrefix(clean, "8") || strings.HasPrefix(clean, "92") {
		exchange = "BSE"
		suffix = "BJ"
	} else {
		exchange = "SZSE"
		suffix = "SZ"
	}
	return map[string]interface{}{
		"instrument_id": fmt.Sprintf("%s.%s", clean, suffix),
		"symbol":        clean,
		"exchange":      exchange,
	}
}

func at(row []interface{}, index int) interface{} {
	if index < len(row) {
		return row[index]
	}
	return nil
}

func parseBoolParam(v interface{}, defaultVal bool) bool {
	if v == nil || valueToString(v) == "" {
		return defaultVal
	}
	if b, ok := v.(bool); ok {
		return b
	}
	text := strings.ToLower(strings.TrimSpace(valueToString(v)))
	if text == "1" || text == "true" || text == "yes" || text == "y" {
		return true
	}
	if text == "0" || text == "false" || text == "no" || text == "n" {
		return false
	}
	return defaultVal
}

func requiredText(params map[string]interface{}, name string) (string, error) {
	v := params[name]
	text := strings.TrimSpace(valueToString(v))
	if text == "" {
		return "", fmt.Errorf("%s is required", name)
	}
	return text, nil
}

func parseBoolValue(v interface{}) interface{} {
	if v == nil || valueToString(v) == "" {
		return nil
	}
	if b, ok := v.(bool); ok {
		return b
	}
	s := strings.TrimSpace(valueToString(v))
	return s == "1" || s == "true" || s == "True"
}

func parseFloat(v interface{}) interface{} {
	text := cleanText(v)
	if text == nil {
		return nil
	}
	t := *text
	t = strings.ReplaceAll(t, ",", "")
	t = strings.ReplaceAll(t, "%", "")
	f, err := strconv.ParseFloat(t, 64)
	if err != nil {
		return nil
	}
	return f
}

func parseInt(v interface{}) interface{} {
	text := cleanText(v)
	if text == nil {
		return nil
	}
	t := *text
	t = strings.ReplaceAll(t, ",", "")
	t = strings.ReplaceAll(t, "%", "")
	f, err := strconv.ParseFloat(t, 64)
	if err != nil {
		return nil
	}
	return int64(f)
}

func cleanText(v interface{}) *string {
	if v == nil {
		return nil
	}
	text := valueToString(v)
	text = strings.ReplaceAll(text, "\t", " ")
	text = strings.ReplaceAll(text, "\n", " ")
	text = strings.ReplaceAll(text, "\r", " ")
	// Collapse multiple spaces
	for strings.Contains(text, "  ") {
		text = strings.ReplaceAll(text, "  ", " ")
	}
	text = strings.TrimSpace(text)
	if text == "" || text == "-" || text == "--" || text == "null" || text == "None" {
		return nil
	}
	return &text
}

func paramAsInt(v interface{}, defaultv int) int {
	switch t := v.(type) {
	case int:
		return t
	case int64:
		return int(t)
	case float64:
		return int(t)
	case float32:
		return int(t)
	case string:
		n, err := strconv.Atoi(strings.TrimSpace(t))
		if err != nil {
			return defaultv
		}
		return n
	default:
		return defaultv
	}
}

func valueToString(v interface{}) string {
	if v == nil {
		return ""
	}
	switch val := v.(type) {
	case string:
		return val
	case float64:
		// Handle JSON unmarshaled numbers
		if val == float64(int64(val)) {
			return strconv.FormatInt(int64(val), 10)
		}
		return strconv.FormatFloat(val, 'f', -1, 64)
	case int:
		return strconv.Itoa(val)
	case int64:
		return strconv.FormatInt(val, 10)
	case bool:
		return strconv.FormatBool(val)
	default:
		return fmt.Sprintf("%v", val)
	}
}
