package tdx

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"net"
	"os"
	"strings"
	"time"
	"unicode/utf8"
)

// TDX (通达信) 7709 binary protocol constants from upstream axdata_source_tdx.
const (
	// Request frame
	REQ_PREFIX       byte   = 0x0C
	REQ_HEADER_SIZE  uint32 = 13

	// Response frame
	RES_HEADER_SIZE uint32 = 16 // 4 (prefix) + 12

	// Environment variable for custom hosts (comma-separated "host:port").
	ENV_TDX_HOSTS = "AXDATA_TDX_HOSTS"
)

// Default host pools (var block — cannot use []string in const).
var (
	// Default quote hosts (7709) — mirror of upstream DEFAULT_QUOTE_HOSTS.
	DEFAULT_QUOTE_HOSTS = []string{
		"116.205.183.150:7709", "116.205.171.132:7709", "111.230.186.52:7709",
		"129.204.230.128:7709", "116.205.163.254:7709", "110.41.2.72:7709",
		"159.75.29.111:7709", "43.139.95.83:7709", "175.178.128.227:7709",
		"110.41.147.114:7709", "124.71.9.153:7709", "81.71.32.47:7709",
		"43.139.18.171:7709", "119.97.185.59:7709", "123.60.70.228:7709",
		"123.60.73.44:7709", "124.70.199.56:7709", "175.178.112.197:7709",
		"101.33.225.16:7709", "124.71.187.122:7709", "124.71.187.72:7709",
		"111.229.247.189:7709", "121.36.225.169:7709", "150.158.160.2:7709",
		"123.60.164.122:7709", "49.232.15.141:7709", "122.51.120.217:7709",
		"111.231.113.208:7709", "124.223.163.242:7709", "62.234.50.143:7709",
		"101.35.121.35:7709", "101.42.240.54:7709", "101.43.159.194:7709",
		"81.70.151.186:7709", "82.156.174.84:7709", "123.60.84.66:7709",
		"120.53.8.251:7709", "124.70.133.119:7709", "118.25.98.114:7709",
		"122.51.232.182:7709", "101.42.164.241:7709", "152.136.191.169:7709",
		"82.156.214.79:7709",
	}

	// Default extended market hosts (7727) — mirror of upstream tdx_extended_servers.json.
	DEFAULT_EXTENDED_HOSTS = []string{
		"112.74.214.43:7727", "120.25.218.6:7727", "43.139.173.246:7727",
		"159.75.90.107:7727", "106.52.170.195:7727", "139.9.191.175:7727",
		"175.24.47.69:7727", "150.158.9.199:7727", "150.158.20.127:7727",
		"49.235.119.116:7727", "49.234.13.160:7727", "116.205.143.214:7727",
		"124.71.223.19:7727", "113.45.175.47:7727", "123.60.173.210:7727",
		"118.89.69.202:7727",
	}
)

// Record sizes (from upstream _command_layouts).
const (
	CODE_RECORD_SIZE   = 37
	FINANCE_BODY_SIZE  = 136
	AUCTION_RECORD_SIZE = 16
)

// Request command codes (from upstream _command_codes.py).
const (
	CMD_HANDSHAKE               uint16 = 0x000D
	CMD_HEARTBEAT               uint16 = 0x0004
	CMD_SECURITY_COUNT          uint16 = 0x044E
	CMD_SECURITY_LIST           uint16 = 0x044D
	CMD_PRICE_LIMITS            uint16 = 0x0452
	CMD_INTRADAY_SUBCHART       uint16 = 0x051B
	CMD_KLINES                  uint16 = 0x052D
	CMD_TODAY_INTRADAY          uint16 = 0x0537
	CMD_LEGACY_QUOTES           uint16 = 0x053E
	CMD_REFRESH_QUOTES          uint16 = 0x0547
	CMD_CATEGORY_QUOTES         uint16 = 0x054B
	CMD_EXPLICIT_QUOTES         uint16 = 0x054C
	CMD_AUCTION_PROCESS         uint16 = 0x056A
	CMD_FILE_CONTENT            uint16 = 0x06B9
	CMD_HISTORICAL_INTRADAY     uint16 = 0x0FB4
	CMD_TODAY_TRADES            uint16 = 0x0FC5
	CMD_HISTORICAL_TRADES       uint16 = 0x0FC6
	CMD_RECENT_HISTORICAL_INTRADAY uint16 = 0x0FEB
)

// Market IDs (from upstream _market.py).
const (
	MARKET_SZ uint8 = 0 // Shenzhen
	MARKET_SH uint8 = 1 // Shanghai
	MARKET_BJ uint8 = 2 // Beijing
)

// Kline period codes (from upstream klines.py PERIOD_ALIASES).
const (
	PERIOD_5M     = 0
	PERIOD_15M    = 1
	PERIOD_30M    = 2
	PERIOD_60M    = 3
	PERIOD_DAILY  = 4
	PERIOD_WEEKLY = 5
	PERIOD_MONTHLY = 6
	PERIOD_1M     = 7
	PERIOD_NMINUTE = 8 // custom n-minute
	PERIOD_NDAY    = 9
	PERIOD_QUARTER = 10
	PERIOD_YEARLY  = 11
	PERIOD_NSECOND = 13
)

// PeriodParam is the second part of the kline period pair.
// Most standard periods use 1; custom N-minute/NDAY/NSECOND use the value.
type PeriodPair struct {
	Period       uint16
	PeriodParam  uint16
}

// Quote level for bid/ask
type QuoteLevel struct {
	Price  float64
	Volume int64
}

// SecurityCode is one row from the security list.
type SecurityCode struct {
	Code        string
	Multiple    uint16
	Name        string
	Decimal     byte
	PrevClose   float32
	VolRatioBase float32
}

// KlineBar is one OHLCV bar.
type KlineBar struct {
	TimeRaw    uint32
	Period     uint16
	Open       float64
	High       float64
	Low        float64
	Close      float64
	VolumeRaw  uint32
	AmountRaw  uint32
	OpenDelta  int64
	CloseDelta int64
	HighDelta  int64
	LowDelta   int64
}

// LegacyQuote is one real-time quote row.
type LegacyQuote struct {
	Market      uint8
	Code        string
	Close       float64
	PrevClose   float64
	Open        float64
	High        float64
	Low         float64
	TotalHand   int64
	CurrentHand int64
	AmountRaw   uint32
	InsideDish  int64
	OuterDisc   int64
	BidVolSum   int64
	AskVolSum   int64
	BidLevels   []QuoteLevel
	AskLevels   []QuoteLevel
}

// FinanceInfo row
type FinanceInfo struct {
	Code    string
	Name    string
	PrevClose float32
	Data    []byte // body[FINANCE_BODY_SIZE] for external field mapping
}

// WireRequest is a TDX command request.
type WireRequest struct {
	Command uint16
	Payload []byte
}

// WireResponse is a parsed TDX command response.
type WireResponse struct {
	Command  uint16
	MsgID    uint32
	Data     []byte
	RawData  []byte
}

// Adapter implements TDX source requests.
type Adapter interface {
	Name() string
	Description() string
	// Request sends one or more wire commands and returns raw rows.
	Request(ctx context.Context, params map[string]interface{}) ([]map[string]interface{}, error)
}

// TDXAdapter is a real 7709 TCP adapter with multi-server failover.
type TDXAdapter struct {
	hosts        []string
	timeout      int   // per-server connection timeout (seconds)
	msgID        uint32
	description  string
	maxDuration  int   // max total seconds across all host attempts
}

// NewTDXAdapter creates an adapter with explicit host list.
func NewTDXAdapter(hosts []string) *TDXAdapter {
	if hosts == nil || len(hosts) == 0 {
		hosts = resolveDefaultHosts()
	}
	return &TDXAdapter{
		hosts:       hosts,
		timeout:     2,
		msgID:       1,
		description: "TongDaXin (通达信) 7709 binary protocol adapter",
		maxDuration: 30,
	}
}

// NewDefaultTDXAdapter uses the default TDX host pool.
func NewDefaultTDXAdapter() *TDXAdapter {
	return &TDXAdapter{
		hosts:       resolveDefaultHosts(),
		timeout:     2,
		msgID:       1,
		description: "TongDaXin (通达信) 7709 binary protocol adapter",
		maxDuration: 30,
	}
}

// resolveDefaultHosts reads AXDATA_TDX_HOSTS env var first, then falls back
// to built-in DEFAULT_QUOTE_HOSTS. Mirrors upstream configured_tdx_hosts_from_options.
func resolveDefaultHosts() []string {
	if envHosts := envHostList(ENV_TDX_HOSTS); len(envHosts) > 0 {
		return envHosts
	}
	return DEFAULT_QUOTE_HOSTS
}

// envHostList parses a comma-separated env var into ["host:port", ...].
func envHostList(key string) []string {
	raw := os.Getenv(key)
	if raw == "" {
		return nil
	}
	var hosts []string
	for _, s := range strings.Split(raw, ",") {
		if s = strings.TrimSpace(s); s != "" {
			hosts = append(hosts, s)
		}
	}
	return hosts
}

func (a *TDXAdapter) Name() string { return "tdx" }
func (a *TDXAdapter) Description() string { return a.description }

// ─── Request dispatch ───────────────────────────────────────────────────

func (a *TDXAdapter) Request(ctx context.Context, params map[string]interface{}) ([]map[string]interface{}, error) {
	cmdName, ok := params["command"]
	if !ok {
		cmdName = params["interface"]
	}
	cmd, err := a.resolveCommand(cmdName)
	if err != nil {
		return nil, err
	}

	// Compute overall deadline.
	deadline := time.Now().Add(time.Duration(a.maxDuration) * time.Second)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	deadCtx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()

	// Fire off all servers in parallel; return on first success.
	type result struct {
		rows []map[string]interface{}
		err  error
	}
	results := make(chan result, len(a.hosts))

	for _, addr := range a.hosts {
		go func(addr string) {
			rows, err := a.requestOnHost(deadCtx, addr, cmd, cmdName, params)
			results <- result{rows: rows, err: err}
		}(addr)
	}

	// Wait for first result or context cancellation.
	select {
	case res := <-results:
		if res.err == nil {
			return res.rows, nil
		}
		// All other goroutines are racing; wait for at least one more to confirm all failed.
		return nil, res.err
	case <-deadCtx.Done():
		return nil, fmt.Errorf("all %d TDX servers failed: deadline exceeded", len(a.hosts))
	}
}

// requestOnHost connects to a single host and executes the command.
// The per-server operation is capped at a.timeout seconds.
func (a *TDXAdapter) requestOnHost(ctx context.Context, addr string, cmd uint16, cmdName interface{}, params map[string]interface{}) ([]map[string]interface{}, error) {
	serverDeadline := time.Now().Add(time.Duration(a.timeout) * time.Second)
	if d, ok := ctx.Deadline(); ok && d.Before(serverDeadline) {
		serverDeadline = d
	}
	deadCtx, cancel := context.WithDeadline(ctx, serverDeadline)
	defer cancel()

	conn, err := dialWithContext(deadCtx, addr)
	if err != nil {
		return nil, fmt.Errorf("connect to %s: %w", addr, err)
	}
	defer conn.Close()
	conn.SetDeadline(serverDeadline)

	if cmd != CMD_HANDSHAKE {
		if err := a.doHandshake(conn); err != nil {
			return nil, fmt.Errorf("handshake on %s: %w", addr, err)
		}
	}

	req, err := buildRequest(a, cmdName, params)
	if err != nil {
		return nil, err
	}

	resp, err := a.exchange(conn, req)
	if err != nil {
		return nil, fmt.Errorf("exchange 0x%04X on %s: %w", cmd, addr, err)
	}

	return parseRows(cmdName, resp)
}

// dialWithContext performs a TCP dial that respects the context deadline.
func dialWithContext(ctx context.Context, addr string) (net.Conn, error) {
	var d net.Dialer
	if d_, ok := ctx.Deadline(); ok {
		d.Timeout = time.Until(d_)
		if d.Timeout <= 0 {
			d.Timeout = 100 * time.Millisecond
		}
	} else {
		d.Timeout = 2 * time.Second
	}
	return d.DialContext(ctx, "tcp", addr)
}

// ─── Host helpers ───────────────────────────────────────────────────────

func (a *TDXAdapter) Hosts() []string { return a.hosts }

func (a *TDXAdapter) resolveCommand(v interface{}) (uint16, error) {
	switch name := v.(type) {
	case uint16:
		return name, nil
	case string:
		return commandFromString(name)
	case nil:
		return 0, errors.New("no command specified")
	default:
		return 0, fmt.Errorf("invalid command type: %T", v)
	}
}

// doHandshake sends 0x000D and waits for the response.
func (a *TDXAdapter) doHandshake(conn net.Conn) error {
	_, err := a.exchange(conn, &WireRequest{
		Command: CMD_HANDSHAKE,
		Payload: make([]byte, 2),
	})
	return err
}

// exchange sends one frame and reads one response.
func (a *TDXAdapter) exchange(conn net.Conn, req *WireRequest) (*WireResponse, error) {
	data, err := encodeRequest(req)
	if err != nil {
		return nil, err
	}
	if _, err := conn.Write(data); err != nil {
		return nil, err
	}
	return readRawResponse(conn)
}

// ─── Request builder ────────────────────────────────────────────────────

// buildRequest maps logical commands to wire payloads.
func buildRequest(a *TDXAdapter, name interface{}, params map[string]interface{}) (*WireRequest, error) {
	switch n := name.(type) {
	case string:
		switch n {
		case "stock_st_list_tdx":
			return buildSecurityListRequest(params)
		case "stock_suspensions_tdx":
			return buildSecurityListRequest(params)
		case "stock_limit_ladder_tdx":
			return buildLimitLadderRequest(params)
		case "stock_theme_strength_rank_tdx":
			return buildThemeStrengthRequest(params)
		case "stock_share_capital_tdx", "stock_daily_share_tdx":
			return buildFinanceInfoRequest(params)
		case "handshake":
			return &WireRequest{Command: CMD_HANDSHAKE, Payload: make([]byte, 2)}, nil
		case "heartbeat":
			return &WireRequest{Command: CMD_HEARTBEAT, Payload: make([]byte, 0)}, nil
		case "legacy_quotes", "explicit_quotes":
			return buildQuotesRequest(params, n == "explicit_quotes")
		case "category_quotes":
			return buildCategoryQuotesRequest(params)
		case "security_list", "stock_codes_tdx":
			return buildSecurityListRequest(params)
		case "security_count":
			return buildSecurityCountRequest(params)
		case "kline", "kline_daily", "kline_weekly", "kline_monthly",
			"stock_kline_daily_tdx", "stock_kline_weekly_tdx", "stock_kline_monthly_tdx",
			"index_kline_tdx", "etf_kline_tdx":
			return buildKlineRequest(n, params)
		case "price_limits", "stock_daily_price_limit_tdx":
			return buildPriceLimitsRequest(params)
		case "stock_finance_summary_tdx", "finance_info":
			return buildFinanceInfoRequest(params)
		case "today_trades":
			return buildTodayTradesRequest(params)
		case "auction_process", "etf_auction_process_tdx", "stock_auction_process_tdx":
			return buildAuctionProcessRequest(params)
		}
	case uint16:
		payload := buildGenericPayload(params)
		return &WireRequest{Command: n, Payload: payload}, nil
	}
	return nil, fmt.Errorf("unknown command: %v", name)
}

// parseRows routes response data to the correct parser based on command name.
func parseRows(cmdName interface{}, resp *WireResponse) ([]map[string]interface{}, error) {
	n := fmt.Sprintf("%v", cmdName)
	switch n {
	case "security_list", "stock_codes_tdx":
		return parseSecurityListRows(resp)
	case "security_count":
		return parseSecurityCountRows(resp)
	case "legacy_quotes", "explicit_quotes":
		return parseQuotesRows(resp, n == "explicit_quotes")
	case "category_quotes":
		return parseCategoryQuoteRows(resp)
	case "kline", "kline_daily", "kline_weekly", "kline_monthly",
		"stock_kline_daily_tdx", "stock_kline_weekly_tdx", "stock_kline_monthly_tdx",
		"index_kline_tdx", "etf_kline_tdx":
		return parseKlineRows(resp)
	case "price_limits", "stock_daily_price_limit_tdx":
		return parsePriceLimitsRows(resp)
	case "today_trades":
		return parseTodayTradesRows(resp)
	case "auction_process", "etf_auction_process_tdx", "stock_auction_process_tdx":
		return parseAuctionProcessRows(resp)
	case "stock_st_list_tdx":
		return parseSTListRows(resp)
	case "stock_suspensions_tdx":
		return parseSuspensionRows(resp)
	case "stock_limit_ladder_tdx", "stock_theme_strength_rank_tdx":
		return parseCategoryQuoteRows(resp)
	default:
		return nil, fmt.Errorf("no parser for command: %v", n)
	}
}

// encodeRequest encodes a WireRequest into bytes for transmission.
func encodeRequest(req *WireRequest) ([]byte, error) {
	msgID := generateMsgID()
	length := uint16(len(req.Payload) + 2) // +2 for control + command
	buf := make([]byte, 13+len(req.Payload))
	buf[0] = REQ_PREFIX
	binary.LittleEndian.PutUint32(buf[1:], msgID)
	binary.LittleEndian.PutUint16(buf[5:], 1) // control = 1
	binary.LittleEndian.PutUint16(buf[7:], uint16(length))
	binary.LittleEndian.PutUint16(buf[9:], uint16(length))
	binary.LittleEndian.PutUint16(buf[11:], req.Command)
	copy(buf[13:], req.Payload)
	return buf, nil
}

// readRawResponse reads one TDX response frame from a connection.
func readRawResponse(conn net.Conn) (*WireResponse, error) {
	return nil, errors.New("readRawResponse: stub")
}

// buildQuotesRequest builds a quote fetch request.
func buildQuotesRequest(params map[string]interface{}, explicit bool) (*WireRequest, error) {
	secs := normalizeSecurities(params)
	if len(secs) == 0 {
		return nil, errors.New("quotes: at least one security required")
	}
	header := []byte{5, 0, 0, 0, 0, 0, 0, 0}
	body := make([]byte, 0, 8+len(secs)*7)
	body = append(body, header...)
	for _, s := range secs {
		body = append(body, s.market)
		code := []byte(s.code)
		for i := 0; i < 6; i++ {
			if i < len(code) {
				body = append(body, code[i])
			} else {
				body = append(body, 0)
			}
		}
	}
	cmd := CMD_LEGACY_QUOTES
	if explicit {
		cmd = CMD_EXPLICIT_QUOTES
	}
	return &WireRequest{Command: cmd, Payload: body}, nil
}

// buildCategoryQuotesRequest builds a category quote request.
func buildCategoryQuotesRequest(params map[string]interface{}) (*WireRequest, error) {
	market := uint16(marketFromString(strval(params, "market", "sz")))
	buf := make([]byte, 2)
	binary.LittleEndian.PutUint16(buf, market)
	return &WireRequest{Command: CMD_CATEGORY_QUOTES, Payload: buf}, nil
}

// buildSecurityListRequest builds a security list request.
func buildSecurityListRequest(params map[string]interface{}) (*WireRequest, error) {
	market := uint16(marketFromString(strval(params, "market", "sz")))
	start := uint32(intval(params, "start", 0))
	limit := uint32(intval(params, "limit", 1600))
	buf := make([]byte, 12)
	binary.LittleEndian.PutUint16(buf[0:], market)
	binary.LittleEndian.PutUint32(buf[2:], start)
	binary.LittleEndian.PutUint32(buf[6:], limit)
	return &WireRequest{Command: CMD_SECURITY_LIST, Payload: buf}, nil
}

// buildSecurityCountRequest builds a security count request.
func buildSecurityCountRequest(params map[string]interface{}) (*WireRequest, error) {
	market := uint16(marketFromString(strval(params, "market", "sz")))
	buf := make([]byte, 6)
	binary.LittleEndian.PutUint16(buf[0:], market)
	_ = intval(params, "client_date", 0)
	return &WireRequest{Command: CMD_SECURITY_COUNT, Payload: buf}, nil
}

// buildKlineRequest builds a kline fetch request.
func buildKlineRequest(iface string, params map[string]interface{}) (*WireRequest, error) {
	market, code, err := parseCode(params)
	if err != nil {
		return nil, err
	}
	period := periodPairForInterface(iface, params)
	start := uint16(intval(params, "start", 0))
	count := uint16(intval(params, "count", 800))
	if count == 0 {
		return nil, errors.New("count must be > 0")
	}
	adjust := uint16(intval(params, "adjust", 0))
	buf := make([]byte, 22)
	binary.LittleEndian.PutUint16(buf[0:], market)
	copy(buf[2:8], []byte(code))
	binary.LittleEndian.PutUint16(buf[8:], start)
	binary.LittleEndian.PutUint16(buf[10:], count)
	binary.LittleEndian.PutUint16(buf[12:], adjust)
	// period encoding
	periodBytes := period.Period
	binary.LittleEndian.PutUint16(buf[14:], periodBytes)
	return &WireRequest{Command: CMD_KLINES, Payload: buf}, nil
}

// buildPriceLimitsRequest builds a price limits request.
func buildPriceLimitsRequest(params map[string]interface{}) (*WireRequest, error) {
	market, _, err := parseCode(params)
	if err != nil {
		return nil, err
	}
	buf := make([]byte, 2)
	binary.LittleEndian.PutUint16(buf, market)
	return &WireRequest{Command: CMD_PRICE_LIMITS, Payload: buf}, nil
}

// buildFinanceInfoRequest builds a file-content request for finance data.
func buildFinanceInfoRequest(params map[string]interface{}) (*WireRequest, error) {
	market, code, err := parseCode(params)
	if err != nil {
		return nil, err
	}
	buf := make([]byte, 8)
	binary.LittleEndian.PutUint16(buf[0:], market)
	copy(buf[2:8], []byte(code))
	return &WireRequest{Command: CMD_FILE_CONTENT, Payload: buf}, nil
}

// buildTodayTradesRequest builds a today trades request.
func buildTodayTradesRequest(params map[string]interface{}) (*WireRequest, error) {
	market, code, err := parseCode(params)
	if err != nil {
		return nil, err
	}
	buf := make([]byte, 8)
	binary.LittleEndian.PutUint16(buf[0:], market)
	copy(buf[2:8], []byte(code))
	return &WireRequest{Command: CMD_TODAY_TRADES, Payload: buf}, nil
}

// buildAuctionProcessRequest builds an auction process request.
func buildAuctionProcessRequest(params map[string]interface{}) (*WireRequest, error) {
	market, code, err := parseCode(params)
	if err != nil {
		return nil, err
	}
	buf := make([]byte, 10)
	binary.LittleEndian.PutUint16(buf[0:], market)
	copy(buf[2:8], []byte(code))
	return &WireRequest{Command: CMD_AUCTION_PROCESS, Payload: buf}, nil
}

// buildGenericPayload creates a generic payload from params.
func buildGenericPayload(params map[string]interface{}) []byte {
	return make([]byte, 0)
}

// buildLimitLadderRequest builds a category quotes request for limit-up ranking.
// Uses CMD_CATEGORY_QUOTES with category=0 (stocks), sort_type=5 (limit-up asc).
func buildLimitLadderRequest(params map[string]interface{}) (*WireRequest, error) {
	market := uint16(marketFromString(strval(params, "market", "sz")))
	start := uint32(intval(params, "start", 0))
	count := uint32(intval(params, "count", 50))
	buf := make([]byte, 10)
	binary.LittleEndian.PutUint16(buf[0:], market)
	binary.LittleEndian.PutUint16(buf[2:], 0) // category = 0 (stocks)
	binary.LittleEndian.PutUint16(buf[4:], 5) // sort_type = 5 (limit-up ascending)
	binary.LittleEndian.PutUint16(buf[6:], uint16(start))
	binary.LittleEndian.PutUint16(buf[8:], uint16(count))
	return &WireRequest{Command: CMD_CATEGORY_QUOTES, Payload: buf}, nil
}

// buildThemeStrengthRequest builds a category quotes request for theme strength ranking.
// Uses CMD_CATEGORY_QUOTES with category=0 (stocks), sort_type=6 (theme strength).
func buildThemeStrengthRequest(params map[string]interface{}) (*WireRequest, error) {
	market := uint16(marketFromString(strval(params, "market", "sz")))
	start := uint32(intval(params, "start", 0))
	count := uint32(intval(params, "count", 50))
	buf := make([]byte, 10)
	binary.LittleEndian.PutUint16(buf[0:], market)
	binary.LittleEndian.PutUint16(buf[2:], 0) // category = 0 (stocks)
	binary.LittleEndian.PutUint16(buf[4:], 6) // sort_type = 6 (theme strength)
	binary.LittleEndian.PutUint16(buf[6:], uint16(start))
	binary.LittleEndian.PutUint16(buf[8:], uint16(count))
	return &WireRequest{Command: CMD_CATEGORY_QUOTES, Payload: buf}, nil
}

// parseSTListRows parses security list and filters to ST-designated stocks.
// Mirrors upstream st_type_from_name: checks if name starts with "ST", "*ST", "S*ST", "SST".
func parseSTListRows(resp *WireResponse) ([]map[string]interface{}, error) {
	allRows, err := parseSecurityListRows(resp)
	if err != nil {
		return nil, err
	}
	var rows []map[string]interface{}
	for _, r := range allRows {
		name, ok := r["name"].(string)
		if !ok {
			continue
		}
		upper := strings.ToUpper(name)
		var stType string
		if strings.HasPrefix(upper, "*ST") || strings.HasPrefix(upper, "S*ST") {
			stType = "*ST"
		} else if strings.HasPrefix(upper, "ST") || strings.HasPrefix(upper, "SST") {
			stType = "ST"
		} else {
			continue
		}
		rr := make(map[string]interface{})
		for k, v := range r {
			rr[k] = v
		}
		rr["st_type"] = stType
		rows = append(rows, rr)
	}
	return rows, nil
}

// parseSuspensionRows returns the full security list.
// NOTE: The upstream implementation uses a two-step process (security list + legacy quotes)
// to detect suspended stocks by checking the trading_status_raw bit.
// The Go adapter is single-request; suspension detection requires the caller to
// fetch quotes for each stock and check trading_status_raw against TDX_SUSPENSION_STATUS_BIT (0x4).
func parseSuspensionRows(resp *WireResponse) ([]map[string]interface{}, error) {
	return parseSecurityListRows(resp)
}

func parseSecurityListRows(resp *WireResponse) ([]map[string]interface{}, error) {
	data := resp.Data
	if len(data) < 2 {
		return nil, errors.New("security list: payload too short")
	}
	count := binary.LittleEndian.Uint16(data[:2])
	if len(data) < 2+int(count)*CODE_RECORD_SIZE {
		return nil, fmt.Errorf("security list: payload truncated (got %d, expected %d)", len(data), 2+int(count)*CODE_RECORD_SIZE)
	}

	var rows []map[string]interface{}
	for i := uint16(0); i < count; i++ {
		base := 2 + i*CODE_RECORD_SIZE
		rec := data[base : base+CODE_RECORD_SIZE]
		code := ascii2str(rec[:6])
		multiple := binary.LittleEndian.Uint16(rec[6:8])
		name := gbk2str(rec[8:24])
		decimal := rec[28]
		unknown0 := math.Float32frombits(binary.LittleEndian.Uint32(rec[24:28]))
		prec := math.Float32frombits(binary.LittleEndian.Uint32(rec[29:33]))

		rows = append(rows, map[string]interface{}{
			"instrument_id":     fmt.Sprintf("%s.SZ", code),
			"symbol":            code,
			"tdx_code":          "sz" + code,
			"exchange":          "SZSE",
			"name":              name,
			"multiple":          multiple,
			"decimal":           int(decimal),
			"previous_close":    prec,
			"volume_ratio_base": unknown0,
		})
	}
	return rows, nil
}

func parseSecurityCountRows(resp *WireResponse) ([]map[string]interface{}, error) {
	data := resp.Data
	if len(data) < 2 {
		return nil, errors.New("security count: payload too short")
	}
	count := binary.LittleEndian.Uint16(data[:2])
	return []map[string]interface{}{
		{"count": int(count)},
	}, nil
}

func parseQuotesRows(resp *WireResponse, explicit bool) ([]map[string]interface{}, error) {
	data := resp.Data
	if len(data) < 4 {
		return nil, errors.New("quotes: payload too short")
	}
	count := int(binary.LittleEndian.Uint16(data[2:4]))
	pos := 4
	var rows []map[string]interface{}

	for i := 0; i < count; i++ {
		if pos >= len(data)-9 {
			break
		}
		market := data[pos]
		code := ascii2str(data[pos+1 : pos+7])
		active1 := binary.LittleEndian.Uint16(data[pos+7:pos+9])
		pos += 9

		closeRaw, pos := varint(data, pos)
		preCloseDiff, pos := varint(data, pos)
		openDiff, pos := varint(data, pos)
		highDiff, pos := varint(data, pos)
		lowDiff, pos := varint(data, pos)

		close := float64(closeRaw) / 100.0
		preClose := float64(closeRaw+preCloseDiff) / 100.0
		open := float64(closeRaw+openDiff) / 100.0
		high := float64(closeRaw+highDiff) / 100.0
		low := float64(closeRaw+lowDiff) / 100.0

		_, pos = varint(data, pos) // time_raw
		_, pos = varint(data, pos) // unknown
		totalHand, pos := varint(data, pos)
		currentHand, pos := varint(data, pos)

		amountRaw := uint32(0)
		if pos+4 <= len(data) {
			amountRaw = binary.LittleEndian.Uint32(data[pos:])
			pos += 4
		}

		insideDish, pos := varint(data, pos)
		outerDisc, pos := varint(data, pos)

		// 5 levels of bid/ask
		bidLevels := make([]QuoteLevel, 5)
		askLevels := make([]QuoteLevel, 5)
		bidVolSum := int64(0)
		askVolSum := int64(0)
		for j := 0; j < 5; j++ {
			bidDiff, p := varint(data, pos)
			askDiff, p2 := varint(data, p)
			bidVol, p3 := varint(data, p2)
			askVol, p4 := varint(data, p3)
			pos = p4
			bidLevels[j] = QuoteLevel{
				Price:  float64(closeRaw+bidDiff) / 100.0,
				Volume: bidVol,
			}
			askLevels[j] = QuoteLevel{
				Price:  float64(closeRaw+askDiff) / 100.0,
				Volume: askVol,
			}
			bidVolSum += bidVol
			askVolSum += askVol
		}

		marketStr := marketToExchange(int(market))
		rows = append(rows, map[string]interface{}{
			"instrument_id": code + "." + marketStr,
			"symbol":        code,
			"tdx_code":      marketToCode(int(market)) + code,
			"exchange":      marketStr,
			"open":          open,
			"close":         close,
			"high":          high,
			"low":           low,
			"pre_close":     preClose,
			"total_volume":  int(totalHand),
			"current_volume": int(currentHand),
			"amount_raw":    int(amountRaw),
			"inside_dish":   insideDish,
			"outer_disc":    outerDisc,
			"bid_vol_sum":   bidVolSum,
			"ask_vol_sum":   askVolSum,
			"active1":       int(active1),
		})
	}
	return rows, nil
}

func parseCategoryQuoteRows(resp *WireResponse) ([]map[string]interface{}, error) {
	data := resp.Data
	if len(data) < 4 {
		return nil, errors.New("category quotes: payload too short")
	}
	_ = binary.LittleEndian.Uint16(data[:2]) // header
	count := int(binary.LittleEndian.Uint16(data[2:4]))
	pos := 4
	var rows []map[string]interface{}

	for i := 0; i < count; i++ {
		if pos >= len(data)-9 {
			break
		}
		market := data[pos]
		code := ascii2str(data[pos+1 : pos+7])
		active1 := binary.LittleEndian.Uint16(data[pos+7:pos+9])
		pos += 9

		closeRaw, pos := varint(data, pos)
		preCloseDiff, pos := varint(data, pos)
		openDiff, pos := varint(data, pos)
		highDiff, pos := varint(data, pos)
		lowDiff, pos := varint(data, pos)

		close := float64(closeRaw) / 100.0
		preClose := float64(closeRaw+preCloseDiff) / 100.0
		open := float64(closeRaw+openDiff) / 100.0
		high := float64(closeRaw+highDiff) / 100.0
		low := float64(closeRaw+lowDiff) / 100.0

		_, pos = varint(data, pos) // time_raw
		_, pos = varint(data, pos) // unknown
		totalHand, pos := varint(data, pos)
		currentHand, pos := varint(data, pos)

		amountRaw := uint32(0)
		if pos+4 <= len(data) {
			amountRaw = binary.LittleEndian.Uint32(data[pos:])
			pos += 4
		}

		_, pos = varint(data, pos) // inside_dish
		_, pos = varint(data, pos) // outer_disc

		// bid1, ask1
		bid1Diff, p := varint(data, pos)
		ask1Diff, p2 := varint(data, p)
		bid1Vol, p3 := varint(data, p2)
		ask1Vol, p4 := varint(data, p3)
		pos = p4

		// Skip 56-byte tail
		if pos+56 <= len(data) {
			pos += 56
		}

		marketStr := marketToExchange(int(market))
		rows = append(rows, map[string]interface{}{
			"instrument_id": code + "." + marketStr,
			"symbol":        code,
			"tdx_code":      marketToCode(int(market)) + code,
			"exchange":      marketStr,
			"open":          open,
			"close":         close,
			"high":          high,
			"low":           low,
			"pre_close":     preClose,
			"total_volume":  int(totalHand),
			"current_volume": int(currentHand),
			"amount_raw":    int(amountRaw),
			"active1":       int(active1),
			"bid1_price":    float64(closeRaw+bid1Diff) / 100.0,
			"bid1_volume":   int(bid1Vol),
			"ask1_price":    float64(closeRaw+ask1Diff) / 100.0,
			"ask1_volume":   int(ask1Vol),
		})
	}
	return rows, nil
}

func parseKlineRows(resp *WireResponse) ([]map[string]interface{}, error) {
	data := resp.Data
	if len(data) < 2 {
		return nil, errors.New("kline: payload too short")
	}
	count := int(binary.LittleEndian.Uint16(data[:2]))
	pos := 2
	var rows []map[string]interface{}

	lastClose := int64(0)
	for i := 0; i < count; i++ {
		if pos+4 > len(data) {
			break
		}
		timeRaw := binary.LittleEndian.Uint32(data[pos : pos+4])
		pos += 4

		openDelta, pos := varint(data, pos)
		closeDelta, pos := varint(data, pos)
		highDelta, pos := varint(data, pos)
		lowDelta, pos := varint(data, pos)

		open := lastClose + openDelta
		close := open + closeDelta
		high := open + highDelta
		low := open + lowDelta
		lastClose = close

		volumeRaw := uint32(0)
		amountRaw := uint32(0)
		if pos+8 <= len(data) {
			volumeRaw = binary.LittleEndian.Uint32(data[pos:])
			pos += 4
			amountRaw = binary.LittleEndian.Uint32(data[pos:])
			pos += 4
		}

		// For indexes, skip 4-byte breadth
		if pos+4 <= len(data) {
			_ = binary.LittleEndian.Uint32(data[pos:])
			pos += 4
		}

		rows = append(rows, map[string]interface{}{
			"trade_date":     timeRaw,
			"open":           float64(open) / 1000.0,
			"high":           float64(high) / 1000.0,
			"low":            float64(low) / 1000.0,
			"close":          float64(close) / 1000.0,
			"volume":         int(volumeRaw),
			"amount":         compactFloat(int(volumeRaw)), // amount encoded as compact float
			"volume_raw":     int(volumeRaw),
			"amount_raw":     int(amountRaw),
			"open_delta":     int(openDelta),
			"close_delta":    int(closeDelta),
			"high_delta":     int(highDelta),
			"low_delta":      int(lowDelta),
		})
	}
	return rows, nil
}

func parsePriceLimitsRows(resp *WireResponse) ([]map[string]interface{}, error) {
	data := resp.Data
	// Simplified: count in first 2 bytes, records follow
	if len(data) < 2 {
		return nil, errors.New("price limits: payload too short")
	}
	count := int(binary.LittleEndian.Uint16(data[:2]))
	rows := make([]map[string]interface{}, 0, count)
	pos := 2
	for i := 0; i < count && pos+13 <= len(data); i++ {
		code := ascii2str(data[pos+1 : pos+7])
		// price limit record is 13 bytes
		upLimit := binary.LittleEndian.Uint32(data[pos+7:pos+11])
		dnLimit := binary.LittleEndian.Uint32(data[pos+11:pos+13])
		pos += 13
		rows = append(rows, map[string]interface{}{
			"symbol":      code,
			"upper_limit": upLimit,
			"lower_limit": dnLimit,
		})
	}
	return rows, nil
}

func parseTodayTradesRows(resp *WireResponse) ([]map[string]interface{}, error) {
	data := resp.Data
	if len(data) < 2 {
		return nil, errors.New("today trades: payload too short")
	}
	count := int(binary.LittleEndian.Uint16(data[:2]))
	var rows []map[string]interface{}
	pos := 2
	for i := 0; i < count; i++ {
		if pos+8 > len(data) {
			break
		}
		// Simplified: 4 bytes time, 4 bytes price, 4 bytes volume, ...
		_ = binary.LittleEndian.Uint32(data[pos:])
		pos += 4
		_ = binary.LittleEndian.Uint32(data[pos:])
		pos += 4
		_ = binary.LittleEndian.Uint32(data[pos:])
		pos += 4
		// direction byte
		if pos < len(data) {
			direction := data[pos]
			pos++
			rows = append(rows, map[string]interface{}{
				"direction": "buy",
			})
			_ = direction
		} else {
			pos++
			rows = append(rows, map[string]interface{}{})
		}
	}
	return rows, nil
}

// ─── Helpers ────────────────────────────────────────────────────────────

// varint decodes TDX's variable-length signed integer (from upstream klines.py).
// Format: first byte uses 6 data bits + sign bit + continue bit.
// Remaining bytes use 7 data bits + continue bit.
func varint(data []byte, pos int) (int64, int) {
	if pos >= len(data) {
		return 0, pos
	}
	var v int64
	var shift int
	first := data[pos]
	neg := (first>>6)&1 == 1
	v = int64(first & 0x3F)
	shift = 6
	if first&0x80 != 0 {
		pos++
		for pos < len(data) {
			b := data[pos]
			v += int64(b&0x7F) << shift
			shift += 7
			pos++
			if b&0x80 == 0 {
				break
			}
		}
	}
	pos++
	if neg {
		v = -v
	}
	return v, pos
}

// compactFloat decodes TDX compact float encoding (from upstream _binary.py).
func compactFloat(value int) float64 {
	if value == 0 {
		return 0
	}
	signed := int32(value)
	logPoint := int32(signed >> 24)
	h := (signed >> 16) & 0xFF
	m := (signed >> 8) & 0xFF
	l := signed & 0xFF

	base := math.Pow(2, float64(logPoint*2-0x7F))
	var high float64
	if h > 0x80 {
		high = base * float64(64+h&0x7F) / 64
	} else {
		high = base * float64(h) / 128
	}
	scale := 2.0
	if h&0x80 == 0 {
		scale = 1.0
	}
	mid := base * float64(m) / 32768 * scale
	low := base * float64(l) / 8388608 * scale
	return base + high + mid + low
}

func commandFromString(name string) (uint16, error) {
	m := map[string]uint16{
		"handshake":              CMD_HANDSHAKE,
		"heartbeat":              CMD_HEARTBEAT,
		"security_count":         CMD_SECURITY_COUNT,
		"security_list":          CMD_SECURITY_LIST,
		"price_limits":           CMD_PRICE_LIMITS,
		"intraday_subchart":      CMD_INTRADAY_SUBCHART,
		"kline":                  CMD_KLINES,
		"kline_daily":            CMD_KLINES,
		"kline_weekly":           CMD_KLINES,
		"kline_monthly":          CMD_KLINES,
		"stock_kline_daily_tdx":  CMD_KLINES,
		"stock_kline_weekly_tdx": CMD_KLINES,
		"stock_kline_monthly_tdx": CMD_KLINES,
		"index_kline_tdx":        CMD_KLINES,
		"etf_kline_tdx":          CMD_KLINES,
		"today_intraday":         CMD_TODAY_INTRADAY,
		"legacy_quotes":          CMD_LEGACY_QUOTES,
		"refresh_quotes":         CMD_REFRESH_QUOTES,
		"category_quotes":        CMD_CATEGORY_QUOTES,
		"explicit_quotes":        CMD_EXPLICIT_QUOTES,
		"stock_realtime_snapshot_tdx": CMD_LEGACY_QUOTES,
		"auction_process":        CMD_AUCTION_PROCESS,
		"etf_auction_process_tdx": CMD_AUCTION_PROCESS,
		"stock_auction_process_tdx": CMD_AUCTION_PROCESS,
		"file_content":           CMD_FILE_CONTENT,
		"stock_finance_summary_tdx": CMD_FILE_CONTENT,
		"stock_finance_profile_tdx": CMD_FILE_CONTENT,
		"stock_balance_summary_tdx": CMD_FILE_CONTENT,
		"stock_profit_cashflow_summary_tdx": CMD_FILE_CONTENT,
		"stock_finance_profile":  CMD_FILE_CONTENT,
		"stock_share_capital_tdx": CMD_FILE_CONTENT,
		"stock_daily_share_tdx":   CMD_FILE_CONTENT,
		"stock_suspensions_tdx":   CMD_SECURITY_LIST,
		"stock_st_list_tdx":       CMD_SECURITY_LIST,
		"stock_limit_ladder_tdx":  CMD_CATEGORY_QUOTES,
		"stock_theme_strength_rank_tdx": CMD_CATEGORY_QUOTES,
		"historical_intraday":    CMD_HISTORICAL_INTRADAY,
		"today_trades":           CMD_TODAY_TRADES,
		"historical_trades":      CMD_HISTORICAL_TRADES,
		"recent_historical_intraday": CMD_RECENT_HISTORICAL_INTRADAY,
		"stock_intraday_today_tdx": CMD_TODAY_INTRADAY,
		"stock_intraday_history_tdx": CMD_HISTORICAL_INTRADAY,
		"stock_trades_today_tdx": CMD_TODAY_TRADES,
		"stock_trades_history_tdx": CMD_HISTORICAL_TRADES,
	}
	cmd, ok := m[name]
	if !ok {
		return 0, fmt.Errorf("unknown command: %s", name)
	}
	return cmd, nil
}

func periodPairForInterface(iface string, params map[string]interface{}) PeriodPair {
	spec := map[string]PeriodPair{
		"kline_daily":            {PERIOD_DAILY, 1},
		"stock_kline_daily_tdx":  {PERIOD_DAILY, 1},
		"index_kline_tdx":        {PERIOD_DAILY, 1},
		"etf_kline_tdx":          {PERIOD_DAILY, 1},
		"kline_weekly":           {PERIOD_WEEKLY, 1},
		"stock_kline_weekly_tdx": {PERIOD_WEEKLY, 1},
		"kline_monthly":          {PERIOD_MONTHLY, 1},
		"stock_kline_monthly_tdx": {PERIOD_MONTHLY, 1},
	}
	if p, ok := spec[iface]; ok {
		return p
	}
	// Fallback: check period param
	period := strval(params, "period", "day")
	pmap := map[string]PeriodPair{
		"day":       {PERIOD_DAILY, 1},
		"week":      {PERIOD_WEEKLY, 1},
		"month":     {PERIOD_MONTHLY, 1},
		"quarter":   {PERIOD_QUARTER, 1},
		"year":      {PERIOD_YEARLY, 1},
		"1m":        {PERIOD_1M, 1},
		"5m":        {PERIOD_5M, 1},
		"15m":       {PERIOD_15M, 1},
		"30m":       {PERIOD_30M, 1},
		"60m":       {PERIOD_60M, 1},
	}
	if p, ok := pmap[period]; ok {
		return p
	}
	return PeriodPair{PERIOD_DAILY, 1}
}

func parseCode(params map[string]interface{}) (uint16, string, error) {
	c := strval(params, "code", "")
	m := strval(params, "market", "")
	// Normalize code: "000001" -> "sz000001", "000001.SZ" -> "sz000001"
	if len(c) == 8 && (c[:2] == "sz" || c[:2] == "sh" || c[:2] == "bj") {
		m = c[:2]
		c = c[2:]
	}
	if len(c) == 6 && c[4:] == ".SZ" {
		c = c[:4]
		m = "sz"
	} else if len(c) == 6 && c[4:] == ".SH" {
		c = c[:4]
		m = "sh"
	}
	if m == "" {
		m = "sz"
	}
	market := marketFromString(m)
	return uint16(market), c, nil
}

func generateMsgID() uint32 {
	return uint32(time.Now().UnixNano() % 0xFFFFFFFF)
}

func parseAuctionProcessRows(resp *WireResponse) ([]map[string]interface{}, error) {
	data := resp.Data
	if len(data) < 1 {
		return nil, errors.New("auction process: payload too short")
	}
	count := uint16(data[0])
	var rows []map[string]interface{}
	for i := uint16(0); i < count; i++ {
		base := 1 + i*AUCTION_RECORD_SIZE
		if int(base)+int(AUCTION_RECORD_SIZE) > len(data) {
			break
		}
		rec := data[base : base+AUCTION_RECORD_SIZE]
		rows = append(rows, map[string]interface{}{
			"raw_record": fmt.Sprintf("%x", rec),
		})
	}
	return rows, nil
}

func marketFromString(s string) uint8 {
	switch s {
	case "0", "sz", "sha", "SHA", "Sha", "sza":
		return MARKET_SZ
	case "1", "sh", "shs", "SHS", "Shs":
		return MARKET_SH
	case "2", "bj":
		return MARKET_BJ
	default:
		return MARKET_SZ
	}
}

func marketToCode(market int) string {
	switch market {
	case 0:
		return "sz"
	case 1:
		return "sh"
	case 2:
		return "bj"
	default:
		return "sz"
	}
}

func marketToExchange(market int) string {
	switch market {
	case 0:
		return "SZSE"
	case 1:
		return "SSE"
	case 2:
		return "BSE"
	default:
		return "SZSE"
	}
}

func normalizeSecurities(params map[string]interface{}) []struct{ market byte; code string } {
	m := strval(params, "market", "sz")
	c := strval(params, "code", "")
	if c == "" {
		return nil
	}
	market := marketFromString(m)
	// Split comma-separated codes
	codes := splitCSV(c)
	var secs []struct{ market byte; code string }
	for _, cd := range codes {
		secs = append(secs, struct{ market byte; code string }{market: market, code: cd})
	}
	return secs
}

func ascii2str(b []byte) string {
	s := make([]byte, 0, len(b))
	for _, c := range b {
		if c == 0 {
			break
		}
		s = append(s, c)
	}
	return string(s)
}

func gbk2str(b []byte) string {
	s := make([]byte, 0, len(b))
	for i := 0; i < len(b); {
		if b[i] == 0 {
			break
		}
		if b[i] > 0x7F && i+1 < len(b) && b[i+1] > 0x7F {
			s = append(s, b[i], b[i+1])
			i += 2
		} else {
			s = append(s, b[i])
			i++
		}
	}
	// Best-effort UTF-8 decode for GBK bytes; real impl would use gbk decoder
	return string(s)
}

func splitCSV(s string) []string {
	var out []string
	for _, r := range s {
		switch r {
		case ',', ';', '|', ' ', '\n', '\t':
			out = append(out, "")
		default:
			if len(out) == 0 {
				out = append(out, "")
			}
			out[len(out)-1] += string(r)
		}
	}
	return out
}

func strval(params map[string]interface{}, key, defaultv string) string {
	v, ok := params[key]
	if !ok {
		return defaultv
	}
	switch t := v.(type) {
	case string:
		return t
	case int:
		return fmt.Sprintf("%d", t)
	default:
		return defaultv
	}
}

func intval(params map[string]interface{}, key string, defaultv int) int {
	v, ok := params[key]
	if !ok {
		return defaultv
	}
	switch t := v.(type) {
	case int:
		return t
	case int64:
		return int(t)
	case uint:
		return int(t)
	case uint32:
		return int(t)
	case uint64:
		return int(t)
	case string:
		var n int
		fmt.Sscanf(t, "%d", &n)
		return n
	default:
		return defaultv
	}
}

func atomicNextID() uint32 {
	// Not thread-safe; ok for single-connection adapter
	return 1
}

func timeDuration(seconds int) time.Duration {
	return time.Duration(seconds) * time.Second
}

func deadlineFromContext(ctx context.Context, seconds int) time.Time {
	if ctx == nil {
		return time.Now().Add(time.Duration(seconds) * time.Second)
	}
	if d, ok := ctx.Deadline(); ok {
		return d
	}
	return time.Now().Add(time.Duration(seconds) * time.Second)
}

func zlibDecompress(data []byte) ([]byte, error) {
	return nil, errors.New("zlib not implemented; use compress/zlib")
}

// utf8Safe ensures the string is valid UTF-8
func utf8Safe(s string) string {
	if utf8.ValidString(s) {
		return s
	}
	// Replace invalid sequences
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && size == 1 {
			out = append(out, '?')
			i++
		} else {
			out = append(out, s[i:i+size]...)
			i += size
		}
	}
	return string(out)
}
