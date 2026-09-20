package tdx

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"net"
	"strings"
	"time"

	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

// ExHq (7727) command codes. Frames carry a 12-byte header like 7709, but
// magic/seq are per-command constants (not 0x010C) and cmds live in 0x23xx.
const (
	CMD_EX_SETUP            uint16 = 0x2454
	CMD_EX_MARKETS          uint16 = 0x23F4
	CMD_EX_INSTRUMENT_COUNT uint16 = 0x23F0
	CMD_EX_INSTRUMENT_INFO  uint16 = 0x23F5
	CMD_EX_INSTRUMENT_BARS  uint16 = 0x23FF
	CMD_EX_INSTRUMENT_QUOTE uint16 = 0x23FA
)

// Kline categories matching pytdx TDXParams.
const (
	EX_KLINE_5MIN    = 0
	EX_KLINE_15MIN   = 1
	EX_KLINE_30MIN   = 2
	EX_KLINE_1HOUR   = 3
	EX_KLINE_DAILY   = 4
	EX_KLINE_WEEKLY  = 5
	EX_KLINE_MONTHLY = 6
	EX_KLINE_1MIN    = 7
	EX_KLINE_1MIN_HQ = 8
	EX_KLINE_RI_K    = 9
	EX_KLINE_3MONTH  = 10
	EX_KLINE_YEARLY  = 11
)

const ENV_TDXEX_HOSTS = "AXDATA_TDXEX_HOSTS"

// exSetupFrame is pytdx ExSetupCmd1 — a 92-byte connect frame
// (12-byte header + 8×8-byte pattern + 16-byte tail).
var exSetupFrame = []byte{
	0x01, 0x01, 0x48, 0x65, 0x00, 0x01, 0x52, 0x00, 0x52, 0x00, 0x54, 0x24,
	0x1f, 0x32, 0xc6, 0xe5, 0xd5, 0x3d, 0xfb, 0x41, 0x1f, 0x32, 0xc6, 0xe5,
	0xd5, 0x3d, 0xfb, 0x41, 0x1f, 0x32, 0xc6, 0xe5, 0xd5, 0x3d, 0xfb, 0x41,
	0x1f, 0x32, 0xc6, 0xe5, 0xd5, 0x3d, 0xfb, 0x41, 0x1f, 0x32, 0xc6, 0xe5,
	0xd5, 0x3d, 0xfb, 0x41, 0x1f, 0x32, 0xc6, 0xe5, 0xd5, 0x3d, 0xfb, 0x41,
	0x1f, 0x32, 0xc6, 0xe5, 0xd5, 0x3d, 0xfb, 0x41, 0x1f, 0x32, 0xc6, 0xe5,
	0xd5, 0x3d, 0xfb, 0x41, 0xcc, 0xe1, 0x6d, 0xff, 0xd5, 0xba, 0x3f, 0xb8,
	0xcb, 0xc5, 0x7a, 0x05, 0x4f, 0x77, 0x48, 0xea,
}

// Complete 12-byte request prefixes from pytdx (header only, payload appended).
var (
	exMarketsFrame = []byte{0x01, 0x02, 0x48, 0x69, 0x00, 0x01, 0x02, 0x00, 0x02, 0x00, 0xf4, 0x23}
	exCountFrame   = []byte{0x01, 0x03, 0x48, 0x66, 0x00, 0x01, 0x02, 0x00, 0x02, 0x00, 0xf0, 0x23}
	exInfoPrefix   = []byte{0x01, 0x04, 0x48, 0x67, 0x00, 0x01, 0x08, 0x00, 0x08, 0x00, 0xf5, 0x23}
	exBarsPrefix   = []byte{0x01, 0x01, 0x08, 0x6a, 0x01, 0x01, 0x16, 0x00, 0x16, 0x00, 0xff, 0x23}
	exQuotePrefix  = []byte{0x01, 0x01, 0x08, 0x02, 0x02, 0x01, 0x0c, 0x00, 0x0c, 0x00, 0xfa, 0x23}
)

// TDXExAdapter talks the 7727 extended-market protocol (pytdx TdxExHq_API).
type TDXExAdapter struct {
	hosts       []string
	timeout     int
	description string
	maxDuration int
}

func NewTDXExAdapter(hosts []string) *TDXExAdapter {
	if hosts == nil || len(hosts) == 0 {
		hosts = resolveExHosts()
	}
	return &TDXExAdapter{
		hosts:       hosts,
		timeout:     2,
		description: "TongDaXin (通达信) 7727 extended-market adapter",
		maxDuration: 30,
	}
}

func NewDefaultTDXExAdapter() *TDXExAdapter {
	return NewTDXExAdapter(nil)
}

func resolveExHosts() []string {
	if envHosts := envHostList(ENV_TDXEX_HOSTS); len(envHosts) > 0 {
		return envHosts
	}
	return DEFAULT_EXTENDED_HOSTS
}

func (a *TDXExAdapter) Name() string        { return "tdxex" }
func (a *TDXExAdapter) Description() string { return a.description }
func (a *TDXExAdapter) Hosts() []string     { return a.hosts }

func (a *TDXExAdapter) Request(ctx context.Context, params map[string]interface{}) ([]map[string]interface{}, error) {
	cmdName, ok := params["command"]
	if !ok {
		cmdName = params["interface"]
	}
	frame, err := buildExFrame(cmdName, params)
	if err != nil {
		return nil, err
	}
	cmd := binary.LittleEndian.Uint16(frame[10:12])

	deadline := time.Now().Add(time.Duration(a.maxDuration) * time.Second)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	deadCtx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()

	if len(a.hosts) == 0 {
		return nil, errors.New("no TDX ExHq hosts configured")
	}

	type result struct {
		rows []map[string]interface{}
		err  error
	}
	results := make(chan result, len(a.hosts))
	for _, addr := range a.hosts {
		go func(addr string) {
			rows, err := a.requestOnHost(deadCtx, addr, cmd, cmdName, params, frame)
			results <- result{rows: rows, err: err}
		}(addr)
	}

	var errs []error
	remaining := len(a.hosts)
	for remaining > 0 {
		select {
		case res := <-results:
			remaining--
			if res.err == nil {
				return res.rows, nil
			}
			errs = append(errs, res.err)
		case <-deadCtx.Done():
			return nil, fmt.Errorf("all %d TDX ExHq servers failed: deadline exceeded", len(a.hosts))
		}
	}
	return nil, fmt.Errorf("all %d TDX ExHq servers failed: %w", len(a.hosts), errors.Join(errs...))
}

func (a *TDXExAdapter) requestOnHost(ctx context.Context, addr string, cmd uint16, cmdName interface{}, params map[string]interface{}, frame []byte) ([]map[string]interface{}, error) {
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

	if err := doExSetup(conn); err != nil {
		return nil, fmt.Errorf("setup on %s: %w", addr, err)
	}
	if _, err := conn.Write(frame); err != nil {
		return nil, fmt.Errorf("write 0x%04X on %s: %w", cmd, addr, err)
	}
	resp, err := readRawResponse(conn, cmd)
	if err != nil {
		return nil, fmt.Errorf("exchange 0x%04X on %s: %w", cmd, addr, err)
	}
	return parseExRows(cmdName, params, resp)
}

func doExSetup(conn net.Conn) error {
	if _, err := conn.Write(exSetupFrame); err != nil {
		return fmt.Errorf("setup write: %w", err)
	}
	if err := drainResponse(conn); err != nil {
		return fmt.Errorf("setup response: %w", err)
	}
	return nil
}

func buildExFrame(name interface{}, params map[string]interface{}) ([]byte, error) {
	n := fmt.Sprintf("%v", name)
	switch n {
	case "markets":
		return append([]byte(nil), exMarketsFrame...), nil
	case "instrument_count":
		return append([]byte(nil), exCountFrame...), nil
	case "instrument_info", "instrument_list":
		return buildExInstrumentInfoFrame(params)
	case "instrument_bars", "kline", "kline_daily",
		"stock_kline_daily_tdxex", "index_kline_tdxex":
		return buildExInstrumentBarsFrame(params)
	case "instrument_quote", "quote":
		return buildExInstrumentQuoteFrame(params)
	default:
		return nil, fmt.Errorf("unknown ExHq command: %v", name)
	}
}

func buildExInstrumentInfoFrame(params map[string]interface{}) ([]byte, error) {
	start := uint32(exInt(params, "start", 0))
	count := uint16(exInt(params, "count", 100))
	extra := make([]byte, 6)
	binary.LittleEndian.PutUint32(extra[0:], start)
	binary.LittleEndian.PutUint16(extra[4:], count)
	return append(append([]byte(nil), exInfoPrefix...), extra...), nil
}

func buildExInstrumentBarsFrame(params map[string]interface{}) ([]byte, error) {
	market, code, err := exMarketCode(params)
	if err != nil {
		return nil, err
	}
	category := uint16(exCategory(params))
	start := uint32(exInt(params, "start", 0))
	count := uint16(exInt(params, "count", 100))
	extra := make([]byte, 20)
	extra[0] = market
	copy(extra[1:10], padCode9(code))
	binary.LittleEndian.PutUint16(extra[10:], category)
	binary.LittleEndian.PutUint16(extra[12:], 1)
	binary.LittleEndian.PutUint32(extra[14:], start)
	binary.LittleEndian.PutUint16(extra[18:], count)
	return append(append([]byte(nil), exBarsPrefix...), extra...), nil
}

func buildExInstrumentQuoteFrame(params map[string]interface{}) ([]byte, error) {
	market, code, err := exMarketCode(params)
	if err != nil {
		return nil, err
	}
	extra := make([]byte, 10)
	extra[0] = market
	copy(extra[1:10], padCode9(code))
	return append(append([]byte(nil), exQuotePrefix...), extra...), nil
}

func parseExRows(cmdName interface{}, params map[string]interface{}, resp *WireResponse) ([]map[string]interface{}, error) {
	n := fmt.Sprintf("%v", cmdName)
	switch n {
	case "markets":
		return parseExMarkets(resp)
	case "instrument_count":
		return parseExInstrumentCount(resp)
	case "instrument_info", "instrument_list":
		return parseExInstrumentInfo(resp)
	case "instrument_bars", "kline", "kline_daily",
		"stock_kline_daily_tdxex", "index_kline_tdxex":
		return parseExInstrumentBars(resp, exCategory(params))
	case "instrument_quote", "quote":
		return parseExInstrumentQuote(resp)
	default:
		return nil, fmt.Errorf("no ExHq parser for command: %v", n)
	}
}

func parseExMarkets(resp *WireResponse) ([]map[string]interface{}, error) {
	data := resp.Data
	if len(data) < 2 {
		return nil, errors.New("ex markets: payload too short")
	}
	cnt := int(binary.LittleEndian.Uint16(data[:2]))
	pos := 2
	var rows []map[string]interface{}
	for i := 0; i < cnt; i++ {
		if pos+64 > len(data) {
			break
		}
		rec := data[pos : pos+64]
		pos += 64
		category := rec[0]
		market := rec[33]
		if category == 0 && market == 0 {
			continue
		}
		rows = append(rows, map[string]interface{}{
			"market":     int(market),
			"category":   int(category),
			"name":       gbkTrim(rec[1:33]),
			"short_name": gbkTrim(rec[34:36]),
		})
	}
	return rows, nil
}

func parseExInstrumentCount(resp *WireResponse) ([]map[string]interface{}, error) {
	data := resp.Data
	if len(data) < 23 {
		return nil, errors.New("ex instrument_count: payload too short")
	}
	num := binary.LittleEndian.Uint32(data[19:23])
	return []map[string]interface{}{{"count": int(num)}}, nil
}

func parseExInstrumentInfo(resp *WireResponse) ([]map[string]interface{}, error) {
	data := resp.Data
	if len(data) < 6 {
		return nil, errors.New("ex instrument_info: payload too short")
	}
	count := int(binary.LittleEndian.Uint16(data[4:6]))
	pos := 6
	var rows []map[string]interface{}
	for i := 0; i < count; i++ {
		if pos+64 > len(data) {
			break
		}
		rec := data[pos : pos+40]
		rows = append(rows, map[string]interface{}{
			"category": int(rec[0]),
			"market":   int(rec[1]),
			"code":     gbkTrim(rec[5:14]),
			"name":     gbkTrim(rec[14:31]),
			"desc":     gbkTrim(rec[31:40]),
		})
		pos += 64
	}
	return rows, nil
}

func parseExInstrumentBars(resp *WireResponse, category int) ([]map[string]interface{}, error) {
	data := resp.Data
	if len(data) < 20 {
		return nil, errors.New("ex instrument_bars: payload too short")
	}
	pos := 18
	retCount := int(binary.LittleEndian.Uint16(data[pos : pos+2]))
	pos += 2
	var rows []map[string]interface{}
	for i := 0; i < retCount; i++ {
		if pos+32 > len(data) {
			break
		}
		year, month, day, hour, minute, np := parseExDatetime(category, data, pos)
		pos = np
		open := f32le(data[pos:])
		high := f32le(data[pos+4:])
		low := f32le(data[pos+8:])
		close := f32le(data[pos+12:])
		position := binary.LittleEndian.Uint32(data[pos+16:])
		trade := binary.LittleEndian.Uint32(data[pos+20:])
		price := f32le(data[pos+24:])
		amount := f32le(data[pos+16:])
		pos += 28
		rows = append(rows, map[string]interface{}{
			"open":       open,
			"high":       high,
			"low":        low,
			"close":      close,
			"position":   int(position),
			"trade":      int(trade),
			"price":      price,
			"amount":     amount,
			"year":       year,
			"month":      month,
			"day":        day,
			"hour":       hour,
			"minute":     minute,
			"datetime":   fmt.Sprintf("%d-%02d-%02d %02d:%02d", year, month, day, hour, minute),
			"trade_date": fmt.Sprintf("%04d%02d%02d", year, month, day),
		})
	}
	return rows, nil
}

func parseExInstrumentQuote(resp *WireResponse) ([]map[string]interface{}, error) {
	data := resp.Data
	if len(data) < 150 {
		return nil, errors.New("ex instrument_quote: payload too short")
	}
	market := int(data[0])
	code := strings.TrimRight(string(data[1:10]), "\x00")
	pos := 14
	return []map[string]interface{}{{
		"market":    market,
		"code":      code,
		"pre_close": f32le(data[pos:]),
		"open":      f32le(data[pos+4:]),
		"high":      f32le(data[pos+8:]),
		"low":       f32le(data[pos+12:]),
		"price":     f32le(data[pos+16:]),
		"kaicang":   int(binary.LittleEndian.Uint32(data[pos+20:])),
		"zongliang": int(binary.LittleEndian.Uint32(data[pos+28:])),
		"xianliang": int(binary.LittleEndian.Uint32(data[pos+32:])),
		"neipan":    int(binary.LittleEndian.Uint32(data[pos+40:])),
		"waipan":    int(binary.LittleEndian.Uint32(data[pos+44:])),
		"chicang":   int(binary.LittleEndian.Uint32(data[pos+52:])),
		"bid1":      f32le(data[pos+56:]),
		"bid2":      f32le(data[pos+60:]),
		"bid3":      f32le(data[pos+64:]),
		"bid4":      f32le(data[pos+68:]),
		"bid5":      f32le(data[pos+72:]),
		"bid_vol1":  int(binary.LittleEndian.Uint32(data[pos+76:])),
		"bid_vol2":  int(binary.LittleEndian.Uint32(data[pos+80:])),
		"bid_vol3":  int(binary.LittleEndian.Uint32(data[pos+84:])),
		"bid_vol4":  int(binary.LittleEndian.Uint32(data[pos+88:])),
		"bid_vol5":  int(binary.LittleEndian.Uint32(data[pos+92:])),
		"ask1":      f32le(data[pos+96:]),
		"ask2":      f32le(data[pos+100:]),
		"ask3":      f32le(data[pos+104:]),
		"ask4":      f32le(data[pos+108:]),
		"ask5":      f32le(data[pos+112:]),
		"ask_vol1":  int(binary.LittleEndian.Uint32(data[pos+116:])),
		"ask_vol2":  int(binary.LittleEndian.Uint32(data[pos+120:])),
		"ask_vol3":  int(binary.LittleEndian.Uint32(data[pos+124:])),
		"ask_vol4":  int(binary.LittleEndian.Uint32(data[pos+128:])),
		"ask_vol5":  int(binary.LittleEndian.Uint32(data[pos+132:])),
	}}, nil
}

func parseExDatetime(category int, data []byte, pos int) (year, month, day, hour, minute, newPos int) {
	if category < 4 || category == 7 || category == 8 {
		zipday := binary.LittleEndian.Uint16(data[pos:])
		tminutes := binary.LittleEndian.Uint16(data[pos+2:])
		year = int(zipday>>11) + 2004
		month = int((zipday % 2048) / 100)
		day = int((zipday % 2048) % 100)
		hour = int(tminutes / 60)
		minute = int(tminutes % 60)
	} else {
		zipday := binary.LittleEndian.Uint32(data[pos:])
		year = int(zipday / 10000)
		month = int((zipday % 10000) / 100)
		day = int(zipday % 100)
		hour = 15
		minute = 0
	}
	return year, month, day, hour, minute, pos + 4
}

func padCode9(code string) []byte {
	b := make([]byte, 9)
	copy(b, []byte(code))
	return b
}

func f32le(b []byte) float64 {
	return float64(math.Float32frombits(binary.LittleEndian.Uint32(b)))
}

func gbkTrim(b []byte) string {
	if n := bytes.IndexByte(b, 0); n >= 0 {
		b = b[:n]
	}
	if len(b) == 0 {
		return ""
	}
	out, _, err := transform.Bytes(simplifiedchinese.GBK.NewDecoder(), b)
	if err != nil {
		return string(b)
	}
	return string(out)
}

func exMarketCode(params map[string]interface{}) (byte, string, error) {
	code := strval(params, "code", "")
	if code == "" {
		code = strval(params, "stock_code", "")
	}
	if code == "" {
		code = strval(params, "symbol", "")
	}
	if code == "" {
		return 0, "", errors.New("ExHq: code is required")
	}
	market := byte(exInt(params, "market", 31))
	return market, code, nil
}

func exCategory(params map[string]interface{}) int {
	if v := exInt(params, "category", -1); v >= 0 {
		return v
	}
	switch strval(params, "period", "day") {
	case "1m":
		return EX_KLINE_1MIN
	case "5m":
		return EX_KLINE_5MIN
	case "15m":
		return EX_KLINE_15MIN
	case "30m":
		return EX_KLINE_30MIN
	case "1h", "60m":
		return EX_KLINE_1HOUR
	case "day":
		return EX_KLINE_DAILY
	case "week":
		return EX_KLINE_WEEKLY
	case "month":
		return EX_KLINE_MONTHLY
	case "year":
		return EX_KLINE_YEARLY
	default:
		return EX_KLINE_DAILY
	}
}

func exInt(params map[string]interface{}, key string, defaultv int) int {
	v, ok := params[key]
	if !ok {
		return defaultv
	}
	switch t := v.(type) {
	case int:
		return t
	case int32:
		return int(t)
	case int64:
		return int(t)
	case uint:
		return int(t)
	case uint32:
		return int(t)
	case uint64:
		return int(t)
	case float64:
		return int(t)
	case string:
		var n int
		fmt.Sscanf(t, "%d", &n)
		return n
	default:
		return defaultv
	}
}
