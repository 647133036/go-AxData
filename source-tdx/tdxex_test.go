package tdx

import (
	"context"
	"encoding/binary"
	"io"
	"math"
	"net"
	"strings"
	"testing"
)

func TestBuildExInstrumentBarsFrame(t *testing.T) {
	frame, err := buildExInstrumentBarsFrame(map[string]interface{}{
		"market": 40, "code": "BABA", "category": 4, "start": 0, "count": 10,
	})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	want := "0101086a010116001600ff232842414241000000000004000100000000000a00"
	got := hexOf(frame)
	if got != want {
		t.Fatalf("frame hex:\n got %s\nwant %s", got, want)
	}
	if binary.LittleEndian.Uint16(frame[10:12]) != CMD_EX_INSTRUMENT_BARS {
		t.Errorf("cmd: got 0x%04X, want 0x%04X", binary.LittleEndian.Uint16(frame[10:12]), CMD_EX_INSTRUMENT_BARS)
	}
}

func TestBuildExInstrumentQuoteFrame(t *testing.T) {
	frame, err := buildExInstrumentQuoteFrame(map[string]interface{}{
		"market": 47, "code": "IF1709",
	})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	want := "0101080202010c000c00fa232f494631373039000000"
	if got := hexOf(frame); got != want {
		t.Fatalf("frame hex:\n got %s\nwant %s", got, want)
	}
}

func TestBuildExInstrumentInfoFrame(t *testing.T) {
	frame, err := buildExInstrumentInfoFrame(map[string]interface{}{
		"start": 0, "count": 100,
	})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	want := "01044867000108000800f523000000006400"
	if got := hexOf(frame); got != want {
		t.Fatalf("frame hex:\n got %s\nwant %s", got, want)
	}
}

func TestParseExInstrumentCount(t *testing.T) {
	body := make([]byte, 23)
	binary.LittleEndian.PutUint32(body[19:], 12345)
	rows, err := parseExInstrumentCount(&WireResponse{Data: body})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if rows[0]["count"].(int) != 12345 {
		t.Fatalf("count: got %v, want 12345", rows[0]["count"])
	}
}

func TestParseExMarkets(t *testing.T) {
	body := make([]byte, 2+64)
	binary.LittleEndian.PutUint16(body[0:], 1)
	body[2] = 1
	copy(body[3:35], []byte("Hong Kong"))
	body[35] = 31
	copy(body[36:38], []byte("HK"))
	rows, err := parseExMarkets(&WireResponse{Data: body})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows: got %d, want 1", len(rows))
	}
	if rows[0]["market"].(int) != 31 {
		t.Errorf("market: got %v", rows[0]["market"])
	}
	if rows[0]["name"].(string) != "Hong Kong" {
		t.Errorf("name: got %q", rows[0]["name"])
	}
}

func TestParseExInstrumentInfo(t *testing.T) {
	body := make([]byte, 6+64*2)
	binary.LittleEndian.PutUint16(body[4:6], 2)
	body[6] = 1
	body[7] = 31
	copy(body[11:20], []byte("IF1709"))
	copy(body[20:37], []byte("沪深1709"))
	copy(body[37:46], []byte("desc1"))
	body[6+64] = 2
	body[6+64+1] = 47
	copy(body[6+64+5:6+64+14], []byte("AU1712"))
	rows, err := parseExInstrumentInfo(&WireResponse{Data: body})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows: got %d, want 2", len(rows))
	}
	if rows[0]["code"].(string) != "IF1709" {
		t.Errorf("code0: got %q", rows[0]["code"])
	}
	if rows[1]["code"].(string) != "AU1712" {
		t.Errorf("code1: got %q", rows[1]["code"])
	}
}

func TestParseExInstrumentInfoTruncated(t *testing.T) {
	body := make([]byte, 6+40)
	binary.LittleEndian.PutUint16(body[4:6], 2)
	body[6] = 1
	body[7] = 31
	copy(body[11:20], []byte("IF1709"))
	rows, err := parseExInstrumentInfo(&WireResponse{Data: body})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("rows: got %d, want 0 (need 64 bytes per record)", len(rows))
	}
}

func TestExCategoryAliases(t *testing.T) {
	cases := []struct {
		period   string
		expected int
	}{
		{"1m", EX_KLINE_1MIN},
		{"5m", EX_KLINE_5MIN},
		{"15m", EX_KLINE_15MIN},
		{"30m", EX_KLINE_30MIN},
		{"1h", EX_KLINE_1HOUR},
		{"60m", EX_KLINE_1HOUR},
		{"day", EX_KLINE_DAILY},
		{"week", EX_KLINE_WEEKLY},
		{"month", EX_KLINE_MONTHLY},
		{"year", EX_KLINE_YEARLY},
		{"unknown", EX_KLINE_DAILY},
	}
	for _, c := range cases {
		got := exCategory(map[string]interface{}{"period": c.period})
		if got != c.expected {
			t.Errorf("period=%q: got %d, want %d", c.period, got, c.expected)
		}
	}
}

func TestExIntTypes(t *testing.T) {
	cases := []struct {
		val      interface{}
		expected int
	}{
		{int(42), 42},
		{int32(42), 42},
		{int64(42), 42},
		{uint(42), 42},
		{uint32(42), 42},
		{uint64(42), 42},
		{float64(42), 42},
		{"42", 42},
		{nil, 99},
	}
	for _, c := range cases {
		params := map[string]interface{}{"k": c.val}
		got := exInt(params, "k", 99)
		if got != c.expected {
			t.Errorf("val=%T(%v): got %d, want %d", c.val, c.val, got, c.expected)
		}
	}
}

func TestParseExInstrumentBarsDaily(t *testing.T) {
	body := make([]byte, 20+32)
	binary.LittleEndian.PutUint16(body[18:], 1)
	binary.LittleEndian.PutUint32(body[20:], 20240115)
	putF32(body[24:], 10.5)
	putF32(body[28:], 11.0)
	putF32(body[32:], 10.0)
	putF32(body[36:], 10.8)
	binary.LittleEndian.PutUint32(body[40:], 100)
	binary.LittleEndian.PutUint32(body[44:], 200)
	putF32(body[48:], 10.7)
	rows, err := parseExInstrumentBars(&WireResponse{Data: body}, EX_KLINE_DAILY)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows: got %d, want 1", len(rows))
	}
	if rows[0]["trade_date"].(string) != "20240115" {
		t.Errorf("trade_date: got %v", rows[0]["trade_date"])
	}
	if rows[0]["open"].(float64) < 10.4 || rows[0]["open"].(float64) > 10.6 {
		t.Errorf("open: got %v", rows[0]["open"])
	}
}

func TestParseExDatetimeIntraday(t *testing.T) {
	buf := make([]byte, 4)
	zipday := uint16((2024-2004)<<11 + 115)
	binary.LittleEndian.PutUint16(buf[0:], zipday)
	binary.LittleEndian.PutUint16(buf[2:], 9*60+30)
	y, m, d, h, min, np := parseExDatetime(EX_KLINE_1MIN, buf, 0)
	if y != 2024 || m != 1 || d != 15 || h != 9 || min != 30 || np != 4 {
		t.Fatalf("got %d-%d-%d %d:%d pos=%d", y, m, d, h, min, np)
	}
}

func drainExSetup(conn net.Conn) error {
	buf := make([]byte, len(exSetupFrame))
	_, err := io.ReadFull(conn, buf)
	return err
}

func TestTDXExRequestRoundTripAgainstLocalServer(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		if err := drainExSetup(conn); err != nil {
			return
		}
		conn.Write(buildTDXReply(CMD_EX_SETUP, []byte{}))
		head := make([]byte, REQ_HEADER_SIZE)
		if _, err := io.ReadFull(conn, head); err != nil {
			return
		}
		payloadLen := int(binary.LittleEndian.Uint16(head[6:8])) - 2
		if payloadLen > 0 {
			payload := make([]byte, payloadLen)
			io.ReadFull(conn, payload)
		}
		body := make([]byte, 23)
		binary.LittleEndian.PutUint32(body[19:], 777)
		conn.Write(buildTDXReply(CMD_EX_INSTRUMENT_COUNT, body))
	}()

	rows, err := NewTDXExAdapter([]string{ln.Addr().String()}).Request(context.Background(), map[string]interface{}{
		"interface": "instrument_count",
	})
	if err != nil {
		t.Fatalf("Request: %v", err)
	}
	if len(rows) != 1 || rows[0]["count"].(int) != 777 {
		t.Fatalf("rows: %+v", rows)
	}
}

func TestTDXExRequestUnknownCommand(t *testing.T) {
	_, err := NewTDXExAdapter([]string{"127.0.0.1:1"}).Request(context.Background(), map[string]interface{}{
		"interface": "not_a_command",
	})
	if err == nil || !strings.Contains(err.Error(), "unknown ExHq command") {
		t.Fatalf("error: %v", err)
	}
}

func TestTDXExRequestFallsOverToSecondHost(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(conn net.Conn) {
				defer conn.Close()
				if err := drainExSetup(conn); err != nil {
					return
				}
				conn.Write(buildTDXReply(CMD_EX_SETUP, []byte{}))
				head := make([]byte, REQ_HEADER_SIZE)
				if _, err := io.ReadFull(conn, head); err != nil {
					return
				}
				payloadLen := int(binary.LittleEndian.Uint16(head[6:8])) - 2
				if payloadLen > 0 {
					payload := make([]byte, payloadLen)
					io.ReadFull(conn, payload)
				}
				body := make([]byte, 23)
				binary.LittleEndian.PutUint32(body[19:], 9)
				conn.Write(buildTDXReply(CMD_EX_INSTRUMENT_COUNT, body))
			}(conn)
		}
	}()

	a := NewTDXExAdapter([]string{"127.0.0.1:1", ln.Addr().String()})
	a.timeout = 1
	a.maxDuration = 5
	rows, err := a.Request(context.Background(), map[string]interface{}{"interface": "instrument_count"})
	if err != nil {
		t.Fatalf("Request: %v", err)
	}
	if rows[0]["count"].(int) != 9 {
		t.Fatalf("count: %+v", rows[0])
	}
}

func hexOf(b []byte) string {
	const hex = "0123456789abcdef"
	out := make([]byte, len(b)*2)
	for i, v := range b {
		out[i*2] = hex[v>>4]
		out[i*2+1] = hex[v&0x0f]
	}
	return string(out)
}

func putF32(b []byte, v float32) {
	binary.LittleEndian.PutUint32(b, math.Float32bits(v))
}
