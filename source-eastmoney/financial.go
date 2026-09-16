package eastmoney

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// Financial statement and valuation endpoints used by the analyst suite.
const (
	EASTMONEY_HSF10_BUSINESS_URL = "https://emweb.securities.eastmoney.com/PC_HSF10/BusinessAnalysis/PageAjax"
)

// requestDataCenter is the shared wrapper for datacenter-web report queries.
func (a *EastMoneyAdapter) requestDataCenter(ctx context.Context, reportName, filter, sortColumns string, page, limit int) ([]map[string]interface{}, error) {
	paramsMap := map[string]string{
		"reportName":  reportName,
		"columns":     "ALL",
		"source":      "WEB",
		"sortColumns": sortColumns,
		"sortTypes":   "-1",
		"pageNumber":  strconv.Itoa(page),
		"pageSize":    strconv.Itoa(limit),
		"filter":      filter,
	}
	data, err := a.httpGet(ctx, buildQueryURL(EASTMONEY_DATA_CENTER_URL, paramsMap),
		"https://data.eastmoney.com/")
	if err != nil {
		return nil, err
	}
	return parseDataCenterRows(data)
}

// secucode normalizes an AxData code into the SECUCODE form, e.g. 000001 -> 000001.SZ.
func secucode(code string) string {
	if strings.Contains(code, ".") {
		return code
	}
	return instrumentID(code)
}

// requestFinancialReport fetches income, balance or cashflow statements.
func (a *EastMoneyAdapter) requestFinancialReport(ctx context.Context, params map[string]interface{}, kind string) ([]map[string]interface{}, error) {
	code := getParamString(params, "code", getParamString(params, "symbol", ""))
	if code == "" {
		return nil, fmt.Errorf("code is required")
	}
	limit := getParamInt(params, "limit", 12)
	if limit > 50 {
		limit = 50
	}

	reportName, columns, mapper := func() (string, string, func(map[string]interface{}) map[string]interface{}) {
		switch kind {
		case "income":
			return "RPT_DMSK_FN_INCOME", "REPORT_DATE", mapIncomeRow
		case "balance":
			return "RPT_DMSK_FN_BALANCE", "REPORT_DATE", mapBalanceRow
		case "cashflow":
			return "RPT_DMSK_FN_CASHFLOW", "REPORT_DATE", mapCashflowRow
		}
		return "", "", nil
	}()
	if mapper == nil {
		return nil, fmt.Errorf("unknown financial report kind: %s", kind)
	}

	rows, err := a.requestDataCenter(ctx, reportName,
		fmt.Sprintf(`(SECUCODE="%s")`, secucode(code)), columns, 1, limit)
	if err != nil {
		return nil, fmt.Errorf("%s statement request: %w", kind, err)
	}

	var out []map[string]interface{}
	for _, r := range rows {
		out = append(out, mapper(r))
	}
	return out, nil
}

func mapIncomeRow(r map[string]interface{}) map[string]interface{} {
	iid := cleanText(r["SECUCODE"])
	sym, exchange := splitInstrumentID(iid)
	revenue := floatVal(r["TOTAL_OPERATE_INCOME"])
	cost := floatVal(r["TOTAL_OPERATE_COST"])
	return map[string]interface{}{
		"ts_code":            iid,
		"symbol":             sym,
		"exchange":           exchange,
		"name":               cleanText(r["SECURITY_NAME_ABBR"]),
		"industry":           cleanText(r["INDUSTRY_NAME"]),
		"report_date":        dateFromValue(r["REPORT_DATE"]),
		"notice_date":        dateFromValue(r["NOTICE_DATE"]),
		"report_type":        cleanText(r["REPORT_TYPE_CODE"]),
		"revenue":            revenue,
		"operating_cost":     cost,
		"total_operate_cost": cost,
		"net_profit":         floatVal(r["PARENT_NETPROFIT"]),
		"net_profit_deduct":  floatVal(r["DEDUCT_PARENT_NETPROFIT"]),
		"operating_profit":   floatVal(r["OPERATE_PROFIT"]),
		"total_profit":       floatVal(r["TOTAL_PROFIT"]),
		"tax":                floatVal(r["INCOME_TAX"]),
		"gross_margin":       grossMargin(revenue, cost),
	}
}

func mapBalanceRow(r map[string]interface{}) map[string]interface{} {
	iid := cleanText(r["SECUCODE"])
	sym, exchange := splitInstrumentID(iid)
	assets := floatVal(r["TOTAL_ASSETS"])
	return map[string]interface{}{
		"ts_code":           iid,
		"symbol":            sym,
		"exchange":          exchange,
		"name":              cleanText(r["SECURITY_NAME_ABBR"]),
		"industry":          cleanText(r["INDUSTRY_NAME"]),
		"report_date":       dateFromValue(r["REPORT_DATE"]),
		"notice_date":       dateFromValue(r["NOTICE_DATE"]),
		"total_assets":      assets,
		"total_liabilities": floatVal(r["TOTAL_LIABILITIES"]),
		"total_equity":      floatVal(r["TOTAL_EQUITY"]),
		// Eastmoney reports both ratios as percentages; normalize to fractions
		// to match the schema convention used by every other ratio column.
		"equity_ratio":     pctToFraction(floatVal(r["TOTAL_EQUITY_RATIO"])),
		"debt_asset_ratio": pctToFraction(floatVal(r["DEBT_ASSET_RATIO"])),
		"leverage_ratio":   leverageRatio(floatVal(r["TOTAL_LIABILITIES"]), assets),
	}
}

// pctToFraction converts a percentage value (90.9 means 90.9%) into a
// fraction (0.909). Empty inputs stay zero.
func pctToFraction(pct float64) float64 {
	return pct / 100
}

func mapCashflowRow(r map[string]interface{}) map[string]interface{} {
	iid := cleanText(r["SECUCODE"])
	sym, exchange := splitInstrumentID(iid)
	ocf := floatVal(r["NETCASH_OPERATE"])
	capex := floatVal(r["CONSTRUCT_LONG_ASSET"])
	return map[string]interface{}{
		"ts_code":          iid,
		"symbol":           sym,
		"exchange":         exchange,
		"name":             cleanText(r["SECURITY_NAME_ABBR"]),
		"industry":         cleanText(r["INDUSTRY_NAME"]),
		"report_date":      dateFromValue(r["REPORT_DATE"]),
		"notice_date":      dateFromValue(r["NOTICE_DATE"]),
		"ocf":              ocf,
		"ocf_ratio":        floatVal(r["NETCASH_OPERATE_RATIO"]),
		"invest_cashflow":  floatVal(r["NETCASH_INVEST"]),
		"finance_cashflow": floatVal(r["NETCASH_FINANCE"]),
		"capex":            capex,
		"free_cash_flow":   ocf - capex,
		"cash_end":         floatVal(r["END_CCE"]),
		"cash_begin":       floatVal(r["BEGIN_CCE"]),
	}
}

func grossMargin(revenue, cost float64) float64 {
	if revenue <= 0 {
		return 0
	}
	return (revenue - cost) / revenue
}

func leverageRatio(liabilities, assets float64) float64 {
	if assets <= 0 {
		return 0
	}
	return liabilities / assets
}

// requestBusinessScope fetches the main-business composition breakdown.
func (a *EastMoneyAdapter) requestBusinessScope(ctx context.Context, params map[string]interface{}) ([]map[string]interface{}, error) {
	code := getParamString(params, "code", getParamString(params, "symbol", ""))
	if code == "" {
		return nil, fmt.Errorf("code is required")
	}
	limit := getParamInt(params, "limit", 100)
	if limit > 500 {
		limit = 500
	}

	emCode := hsf10Code(code)
	u := EASTMONEY_HSF10_BUSINESS_URL + "?code=" + emCode
	data, err := a.httpGet(ctx, u, "https://emweb.securities.eastmoney.com/")
	if err != nil {
		return nil, fmt.Errorf("business scope request: %w", err)
	}

	var raw struct {
		Zygcfx []map[string]interface{} `json:"zygcfx"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("business scope parse: %w", err)
	}

	var out []map[string]interface{}
	for _, r := range raw.Zygcfx {
		if len(out) >= limit {
			break
		}
		out = append(out, mapBusinessRow(r))
	}
	return out, nil
}

func mapBusinessRow(r map[string]interface{}) map[string]interface{} {
	iid := cleanText(r["SECUCODE"])
	sym, exchange := splitInstrumentID(iid)
	income := floatVal(r["MAIN_BUSINESS_INCOME"])
	ratio := floatVal(r["MBI_RATIO"])
	profit := floatVal(r["MAIN_BUSINESS_RPOFIT"])
	return map[string]interface{}{
		"ts_code":            iid,
		"symbol":             sym,
		"exchange":           exchange,
		"report_date":        dateFromValue(r["REPORT_DATE"]),
		"mainop_type":        cleanText(r["MAINOP_TYPE"]),
		"item_name":          cleanText(r["ITEM_NAME"]),
		"income":             income,
		"income_ratio":       ratio,
		"cost":               floatVal(r["MAIN_BUSINESS_COST"]),
		"profit":             profit,
		"gross_profit_ratio": floatVal(r["GROSS_RPOFIT_RATIO"]),
		"rank":               int(floatVal(r["RANK"])),
	}
}

// hsf10Code converts an AxData code into the emweb form, e.g. 000001.SZ -> SZ000001.
// hsf10Code converts an AxData instrument ID to the SECUCODE-style prefix the
// HSF10 endpoints expect, e.g. 600519.SH -> SH600519.
func hsf10Code(code string) string {
	sym, exchange := splitInstrumentID(secucode(code))
	if sym == "" {
		return code
	}
	prefix := ""
	switch exchange {
	case "SSE":
		prefix = "SH"
	case "SZSE":
		prefix = "SZ"
	case "BSE":
		prefix = "BJ"
	}
	if prefix == "" {
		return code
	}
	return prefix + sym
}

// requestEarningsForecast fetches profit pre-announcement records.
func (a *EastMoneyAdapter) requestEarningsForecast(ctx context.Context, params map[string]interface{}) ([]map[string]interface{}, error) {
	code := getParamString(params, "code", getParamString(params, "symbol", ""))
	if code == "" {
		return nil, fmt.Errorf("code is required")
	}
	limit := getParamInt(params, "limit", 20)
	if limit > 100 {
		limit = 100
	}

	rows, err := a.requestDataCenter(ctx, "RPT_PUBLIC_OP_NEWPREDICT",
		fmt.Sprintf(`(SECURITY_CODE="%s")`, bareSymbol(code)), "NOTICE_DATE", 1, limit)
	if err != nil {
		return nil, fmt.Errorf("earnings forecast request: %w", err)
	}

	var out []map[string]interface{}
	for _, r := range rows {
		out = append(out, mapForecastRow(r))
	}
	return out, nil
}

func mapForecastRow(r map[string]interface{}) map[string]interface{} {
	iid := cleanText(r["SECUCODE"])
	sym, exchange := splitInstrumentID(iid)
	return map[string]interface{}{
		"ts_code":       iid,
		"symbol":        sym,
		"exchange":      exchange,
		"name":          cleanText(r["SECURITY_NAME_ABBR"]),
		"notice_date":   dateFromValue(r["NOTICE_DATE"]),
		"report_date":   dateFromValue(r["REPORT_DATE"]),
		"forecast_item": cleanText(r["PREDICT_FINANCE"]),
		"forecast_type": cleanText(r["PREDICT_TYPE"]),
		"amount_lower":  floatVal(r["PREDICT_AMT_LOWER"]),
		"amount_upper":  floatVal(r["PREDICT_AMT_UPPER"]),
		"change_lower":  floatVal(r["ADD_AMP_LOWER"]),
		"change_upper":  floatVal(r["ADD_AMP_UPPER"]),
		"reason":        cleanText(r["PREDICT_CONTENT"]),
		"report_period": dateFromValue(r["REPORT_DATE"]),
	}
}

// requestValuationSnapshot fetches daily valuation metrics.
func (a *EastMoneyAdapter) requestValuationSnapshot(ctx context.Context, params map[string]interface{}) ([]map[string]interface{}, error) {
	code := getParamString(params, "code", getParamString(params, "symbol", ""))
	if code == "" {
		return nil, fmt.Errorf("code is required")
	}
	limit := getParamInt(params, "limit", 1)
	if limit > 100 {
		limit = 100
	}

	rows, err := a.requestDataCenter(ctx, "RPT_VALUEANALYSIS_DET",
		fmt.Sprintf(`(SECURITY_CODE="%s")`, bareSymbol(code)), "TRADE_DATE", 1, limit)
	if err != nil {
		return nil, fmt.Errorf("valuation snapshot request: %w", err)
	}

	var out []map[string]interface{}
	for _, r := range rows {
		out = append(out, mapValuationRow(r))
	}
	return out, nil
}

func mapValuationRow(r map[string]interface{}) map[string]interface{} {
	iid := cleanText(r["SECUCODE"])
	sym, exchange := splitInstrumentID(iid)
	return map[string]interface{}{
		"ts_code":          iid,
		"symbol":           sym,
		"exchange":         exchange,
		"name":             cleanText(r["SECURITY_NAME_ABBR"]),
		"industry":         cleanText(r["BOARD_NAME"]),
		"trade_date":       dateFromValue(r["TRADE_DATE"]),
		"close_price":      floatVal(r["CLOSE_PRICE"]),
		"change_pct":       floatVal(r["CHANGE_RATE"]),
		"total_market_cap": floatVal(r["TOTAL_MARKET_CAP"]),
		"free_market_cap":  floatVal(r["NOTLIMITED_MARKETCAP_A"]),
		"total_shares":     floatVal(r["TOTAL_SHARES"]),
		"free_shares":      floatVal(r["FREE_SHARES_A"]),
		"pe_ttm":           floatVal(r["PE_TTM"]),
		"pe_lar":           floatVal(r["PE_LAR"]),
		"pb":               floatVal(r["PB_MRQ"]),
		"ps_ttm":           floatVal(r["PS_TTM"]),
		"pcf_ttm":          floatVal(r["PCF_OCF_TTM"]),
		"peg":              floatVal(r["PEG_CAR"]),
	}
}

// bareSymbol strips the exchange suffix, used by SECURITY_CODE filters.
func bareSymbol(code string) string {
	if i := strings.Index(code, "."); i > 0 {
		return code[:i]
	}
	return code
}
