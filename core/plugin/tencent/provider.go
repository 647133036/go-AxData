package tencent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/electkismet/axdata-go/core/plugin"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

// TencentProvider implements the AxData SourceProvider for Tencent Finance API.
type TencentProvider struct {
	providerID   string
	sourceCode   string
	sourceNameZh string
	version      string
}

// NewProvider creates a new Tencent provider.
func NewProvider() plugin.SourceProvider {
	return &TencentProvider{
		providerID:   "axdata.source.tencent",
		sourceCode:   "tencent",
		sourceNameZh: "腾讯财经",
		version:      "0.1.0",
	}
}

// ProviderID returns the globally unique provider identifier.
func (p *TencentProvider) ProviderID() string {
	return p.providerID
}

// SourceCode returns the short source code key.
func (p *TencentProvider) SourceCode() string {
	return p.sourceCode
}

// SourceNameZh returns the Chinese display name.
func (p *TencentProvider) SourceNameZh() string {
	return p.sourceNameZh
}

// Version returns the provider version.
func (p *TencentProvider) Version() string {
	return p.version
}

// Interfaces returns the full interface catalog.
func (p *TencentProvider) Interfaces() []plugin.SourceInterface {
	return INTERFACES
}

// CreateAdapter creates a new Tencent adapter instance.
func (p *TencentProvider) CreateAdapter(options map[string]interface{}) plugin.SourceAdapter {
	return &TencentAdapter{
		baseURL:   "https://qt.gtimg.cn",
		client:    &http.Client{Timeout: 30 * time.Second},
		description: "腾讯财经 (Tencent Finance) API adapter - 实时行情与K线数据",
	}
}

// ─── Adapter ──────────────────────────────────────────────────────────

// TencentAdapter implements the SourceAdapter for Tencent Finance API.
type TencentAdapter struct {
	baseURL     string
	client      *http.Client
	description string
}

// Name returns the adapter name.
func (a *TencentAdapter) Name() string {
	return "tencent"
}

// Description returns adapter description.
func (a *TencentAdapter) Description() string {
	return a.description
}

// Request executes a source request.
func (a *TencentAdapter) Request(ctx context.Context, params map[string]interface{}) ([]map[string]interface{}, error) {
	interfaceName, _ := params["interface"].(string)
	symbols, _ := params["symbols"].(string)

	if symbols == "" {
		return nil, fmt.Errorf("symbols parameter required")
	}

	switch interfaceName {
	case "stock_quote_tencent":
		return a.requestQuote(ctx, symbols)
	case "stock_kline_tencent":
		return a.requestKline(ctx, symbols, params)
	case "index_quote_tencent":
		return a.requestQuote(ctx, symbols)
	default:
		return nil, fmt.Errorf("unknown interface: %s", interfaceName)
	}
}

// requestQuote fetches real-time quote data.
func (a *TencentAdapter) requestQuote(ctx context.Context, symbols string) ([]map[string]interface{}, error) {
	symStr := tencentFormatSymbols(symbols)
	apiURL := fmt.Sprintf("%s/q=%s", a.baseURL, symStr)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Referer", "https://finance.qq.com")

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("tencent request: %w", err)
	}
	defer resp.Body.Close()

	data, err := a.readAll(resp)
	if err != nil {
		return nil, err
	}

	return a.parseQuoteString(string(data))
}

// requestKline fetches K-line data.
func (a *TencentAdapter) requestKline(ctx context.Context, symbol string, params map[string]interface{}) ([]map[string]interface{}, error) {
	s := tencentCleanSymbol(symbol)
	period := "101"
	if p, ok := params["period"].(string); ok {
		period = p
	}
	adjust := "1"
	if ad, ok := params["adjust"].(string); ok {
		adjust = ad
	}
	limit := "500"
	if l, ok := params["limit"].(string); ok {
		limit = l
	}

	apiURL := fmt.Sprintf(
		"https://web.ifzq.gtimg.cn/appstock/app/fqkline/get?param=%s,%s,%s,%s,qfq",
		s, period, adjust, limit,
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Referer", "https://stockapp.finance.qq.com")
	req.Header.Set("User-Agent", "Mozilla/5.0")

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("kline request: %w", err)
	}
	defer resp.Body.Close()

	data, err := a.readAll(resp)
	if err != nil {
		return nil, err
	}

	return a.parseKlineJSON(data)
}

// readAll reads and decodes GBK response.
func (a *TencentAdapter) readAll(resp *http.Response) ([]byte, error) {
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	decoded, err := decodeGBK(raw)
	if err != nil {
		decoded = raw
	}
	return decoded, nil
}

// parseQuoteString parses Tencent quote response.
func (a *TencentAdapter) parseQuoteString(raw string) ([]map[string]interface{}, error) {
	var results []map[string]interface{}

	lines := strings.Split(raw, "\n")
	for _, line := range lines {
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		data := strings.Trim(parts[1], "\"")
		fields := strings.Split(data, "~")
		if len(fields) < 30 {
			continue
		}

		code := tencentExtractCode(parts[0])
		axCode := tencentCodeToAxCode(code)

		record := map[string]interface{}{
			"instrument_id":  axCode,
			"code":           code,
			"name":           fields[1],
			"close":          fields[3],
			"high":           fields[4],
			"low":            fields[33],
			"open":           fields[5],
			"volume":         fields[6],
			"buy_vol":        fields[7],
			"sell_vol":       fields[8],
			"bid_1":          fields[9],
			"bid_1_vol":      fields[10],
			"ask_1":          fields[11],
			"ask_1_vol":      fields[12],
			"bid_2":          fields[13],
			"bid_2_vol":      fields[14],
			"ask_2":          fields[15],
			"ask_2_vol":      fields[16],
			"bid_3":          fields[17],
			"bid_3_vol":      fields[18],
			"ask_3":          fields[19],
			"ask_3_vol":      fields[20],
			"bid_4":          fields[21],
			"bid_4_vol":      fields[22],
			"ask_4":          fields[23],
			"ask_4_vol":      fields[24],
			"bid_5":          fields[25],
			"bid_5_vol":      fields[26],
			"ask_5":          fields[27],
			"ask_5_vol":      fields[28],
			"trade_time":     fields[29],
			"change":         fields[30],
			"change_pct":     fields[31],
			"pre_close":      fields[32],
		}
		results = append(results, record)
	}

	return results, nil
}

// parseKlineJSON parses Tencent K-line JSON.
func (a *TencentAdapter) parseKlineJSON(data []byte) ([]map[string]interface{}, error) {
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse kline JSON: %w", err)
	}

	if items, ok := raw["data"]; ok {
		if q, ok := items.(map[string]interface{}); ok {
			// Try daily klines first
			for key, val := range q {
				if !strings.Contains(key, "daily") && !strings.Contains(key, "qfq") {
					continue
				}
				if arr, ok := val.([]interface{}); ok {
					return a.parseKlineArray(arr), nil
				}
			}
		}
	}

	return nil, fmt.Errorf("kline data not found in response (keys: %v)",
		func() []string {
			var k []string
			if items, ok := raw["data"]; ok {
				if q, ok := items.(map[string]interface{}); ok {
					for key := range q {
						k = append(k, key)
					}
				}
			}
			return k
		}())
}

// parseKlineArray parses a K-line array.
func (a *TencentAdapter) parseKlineArray(arr []interface{}) []map[string]interface{} {
	var results []map[string]interface{}
	for _, item := range arr {
		if s, ok := item.(string); ok {
			fields := strings.Split(s, ",")
			if len(fields) < 6 {
				continue
			}
			results = append(results, map[string]interface{}{
				"trade_date": fields[0],
				"open":       fields[1],
				"close":      fields[2],
				"high":       fields[3],
				"low":        fields[4],
				"volume":     fields[5],
				"amount":     fields[6],
			})
		}
	}
	return results
}

// ─── Helpers ──────────────────────────────────────────────────────────

// tencentFormatSymbols formats symbols for Tencent API.
func tencentFormatSymbols(symbols string) string {
	parts := strings.Split(symbols, ",")
	formatted := []string{}
	for _, p := range parts {
		s := tencentCleanSymbol(p)
		if s != "" {
			formatted = append(formatted, s)
		}
	}
	return strings.Join(formatted, ",")
}

// tencentCleanSymbol converts AxData symbol to Tencent format.
func tencentCleanSymbol(symbol string) string {
	s := strings.ToLower(strings.TrimSpace(symbol))
	s = strings.ReplaceAll(s, ".sz", "")
	s = strings.ReplaceAll(s, ".sh", "")
	s = strings.ReplaceAll(s, ".bj", "")

	if !strings.HasPrefix(s, "sz") && !strings.HasPrefix(s, "sh") && !strings.HasPrefix(s, "bj") {
		if strings.HasPrefix(s, "0") || strings.HasPrefix(s, "1") || strings.HasPrefix(s, "2") || strings.HasPrefix(s, "3") {
			s = "sz" + s
		} else if strings.HasPrefix(s, "6") {
			s = "sh" + s
		} else if strings.HasPrefix(s, "9") || strings.HasPrefix(s, "4") || strings.HasPrefix(s, "8") {
			s = "bj" + s
		}
	}
	return s
}

// tencentExtractCode extracts code from variable name.
func tencentExtractCode(varName string) string {
	return strings.ReplaceAll(varName, "v_", "")
}

// tencentCodeToAxCode converts Tencent code to AxData format.
func tencentCodeToAxCode(code string) string {
	s := strings.ToLower(code)
	if strings.HasPrefix(s, "sz") {
		return strings.TrimPrefix(s, "sz") + ".SZ"
	}
	if strings.HasPrefix(s, "sh") {
		return strings.TrimPrefix(s, "sh") + ".SH"
	}
	if strings.HasPrefix(s, "bj") {
		return strings.TrimPrefix(s, "bj") + ".BJ"
	}
	return s
}

// decodeGBK decodes GBK bytes to UTF-8.
func decodeGBK(data []byte) ([]byte, error) {
	decoder := simplifiedchinese.GBK.NewDecoder()
	decoded, _, err := transform.Bytes(decoder, data)
	if err != nil {
		return nil, fmt.Errorf("decode GBK: %w", err)
	}
	return decoded, nil
}
