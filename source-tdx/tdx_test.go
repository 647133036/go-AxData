package tdx

import (
	"bytes"
	"compress/flate"
	"compress/zlib"
	"context"
	"encoding/binary"
	"io"
	"net"
	"strings"
	"testing"
	"time"
)

// buildTDXReply encodes a 7709 reply frame the way a server would.
func buildTDXReply(command uint16, control uint16, data []byte) []byte {
	buf := make([]byte, int(RES_HEADER_SIZE)+len(data))
	binary.LittleEndian.PutUint32(buf[1:], 0x11223344)
	binary.LittleEndian.PutUint16(buf[5:], control)
	binary.LittleEndian.PutUint16(buf[7:], uint16(len(data)))
	binary.LittleEndian.PutUint16(buf[9:], uint16(len(data)+2))
	binary.LittleEndian.PutUint16(buf[11:], command)
	copy(buf[RES_HEADER_SIZE:], data)
	return buf
}

// TestTDXRequestRoundTripAgainstLocalServer drives a real Request() through a
// local server that speaks the wire protocol, proving the handshake, the
// request frame, the response decoder and the parser all agree.
func TestTDXRequestRoundTripAgainstLocalServer(t *testing.T) {
	want := uint16(3)
	seen := make(chan uint16, 8)

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
		for {
			head := make([]byte, RES_HEADER_SIZE)
			if _, err := io.ReadFull(conn, head); err != nil {
				return
			}
			cmd := binary.LittleEndian.Uint16(head[11:13])
			payloadLen := int(binary.LittleEndian.Uint16(head[7:9]))
			payload := make([]byte, payloadLen)
			if _, err := io.ReadFull(conn, payload); err != nil {
				return
			}
			seen <- cmd
			if cmd == CMD_HANDSHAKE {
				conn.Write(buildTDXReply(CMD_HANDSHAKE, RESP_CONTROL_PLAIN, make([]byte, 2)))
			} else {
				body := make([]byte, 2)
				binary.LittleEndian.PutUint16(body, want)
				conn.Write(buildTDXReply(CMD_SECURITY_COUNT, RESP_CONTROL_PLAIN, body))
			}
		}
	}()

	rows, err := NewTDXAdapter([]string{ln.Addr().String()}).Request(context.Background(), map[string]interface{}{
		"interface": "security_count",
		"market":    "sz",
	})
	if err != nil {
		t.Fatalf("Request: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows: got %d, want 1: %v", len(rows), rows)
	}
	if got := rows[0]["count"].(int); got != int(want) {
		t.Fatalf("count: got %d, want %d", got, want)
	}

	// Handshake must precede the data request on the same connection.
	if got := <-seen; got != CMD_HANDSHAKE {
		t.Errorf("first command: got 0x%04X, want 0x%04X", got, CMD_HANDSHAKE)
	}
	if got := <-seen; got != CMD_SECURITY_COUNT {
		t.Errorf("second command: got 0x%04X, want 0x%04X", got, CMD_SECURITY_COUNT)
	}
}

// TestTDXRequestFailsFastOnSilentServer pins the failure mode when a server
// accepts and never replies: the block happens in the header read, it is
// surfaced as a read-path error, and it is bounded by the per-server deadline.
func TestTDXRequestFailsFastOnSilentServer(t *testing.T) {
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
			go func() {
				<-time.After(5 * time.Second)
				conn.Close()
			}()
		}
	}()

	a := NewTDXAdapter([]string{ln.Addr().String()})
	start := time.Now()
	_, err = a.Request(context.Background(), map[string]interface{}{"interface": "security_count"})
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected an error from a silent server")
	}
	if !strings.Contains(err.Error(), "reading response header") {
		t.Errorf("error %q does not name the read path", err.Error())
	}
	if elapsed > 5*time.Second {
		t.Errorf("failed after %v; expected a fast failure", elapsed)
	}
}

// TestEncodeRequestLengthMatchesFrame pins the frame-length invariant. A
// conforming server reads the header's data_len bytes after the header, so if
// data_len exceeds the bytes actually sent the server reads into the next
// frame and the whole session desyncs.
func TestEncodeRequestLengthMatchesFrame(t *testing.T) {
	frame, err := encodeRequest(&WireRequest{Command: CMD_SECURITY_COUNT, Payload: []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00}})
	if err != nil {
		t.Fatalf("encodeRequest: %v", err)
	}
	if len(frame) != int(REQ_HEADER_SIZE)+6 {
		t.Fatalf("frame len: got %d, want %d", len(frame), int(REQ_HEADER_SIZE)+6)
	}
	dataLen := int(binary.LittleEndian.Uint16(frame[7:9]))
	dataLenPlus := int(binary.LittleEndian.Uint16(frame[9:11]))
	if dataLen != 6 {
		t.Errorf("data_len: got %d, want 6", dataLen)
	}
	if dataLenPlus != 8 {
		t.Errorf("data_len+2: got %d, want 8", dataLenPlus)
	}
	if dataLen != len(frame)-int(REQ_HEADER_SIZE) {
		t.Errorf("data_len %d does not match the %d payload bytes sent", dataLen, len(frame)-int(REQ_HEADER_SIZE))
	}
}

// pipeWith hands frame to the reader half and closes the writer half, so a
// short frame surfaces as EOF instead of blocking ReadFull forever.
func pipeWith(frame []byte) (io.Reader, func()) {
	server, client := net.Pipe()
	go func() {
		server.Write(frame)
		server.Close()
	}()
	return client, func() { client.Close() }
}

func TestReadRawResponsePlain(t *testing.T) {
	payload := []byte{0x03, 0x00} // uint16(3)
	frame := buildTDXReply(CMD_SECURITY_COUNT, RESP_CONTROL_PLAIN, payload)

	client, close := pipeWith(frame)
	defer close()

	resp, err := readRawResponse(client, CMD_SECURITY_COUNT)
	if err != nil {
		t.Fatalf("readRawResponse: %v", err)
	}
	if resp.Command != CMD_SECURITY_COUNT {
		t.Errorf("Command: got 0x%04X, want 0x%04X", resp.Command, CMD_SECURITY_COUNT)
	}
	if resp.MsgID != 0x11223344 {
		t.Errorf("MsgID: got 0x%08X, want 0x11223344", resp.MsgID)
	}
	if !bytes.Equal(resp.Data, payload) {
		t.Errorf("Data: got %v, want %v", resp.Data, payload)
	}
	if len(resp.RawData) != int(RES_HEADER_SIZE)+len(payload) {
		t.Errorf("RawData len: got %d, want %d", len(resp.RawData), int(RES_HEADER_SIZE)+len(payload))
	}
	if binary.LittleEndian.Uint16(resp.Data) != 3 {
		t.Errorf("payload parses as %d, want 3", binary.LittleEndian.Uint16(resp.Data))
	}
}

func TestReadRawResponseCompressed(t *testing.T) {
	payload := []byte{0x07, 0x00}
	var buf bytes.Buffer
	zw := zlib.NewWriter(&buf)
	if _, err := zw.Write(payload); err != nil {
		t.Fatalf("compress: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	frame := buildTDXReply(CMD_SECURITY_COUNT, RESP_CONTROL_COMPRESSED, buf.Bytes())

	client, close := pipeWith(frame)
	defer close()

	resp, err := readRawResponse(client, CMD_SECURITY_COUNT)
	if err != nil {
		t.Fatalf("readRawResponse: %v", err)
	}
	if !bytes.Equal(resp.Data, payload) {
		t.Errorf("Data: got %v, want %v", resp.Data, payload)
	}
	if len(resp.RawData) != int(RES_HEADER_SIZE)+buf.Len() {
		t.Errorf("RawData len: got %d, want %d (compressed wire bytes)", len(resp.RawData), int(RES_HEADER_SIZE)+buf.Len())
	}
}

func TestReadRawResponseFailures(t *testing.T) {
	base := buildTDXReply(CMD_SECURITY_COUNT, RESP_CONTROL_PLAIN, []byte{0x01, 0x00})
	overclaim := putUint16(base, 7, 4) // declares 4 payload bytes, only 2 follow

	cases := []struct {
		name    string
		frame   []byte
		wantCmd uint16
		wantErr string
	}{
		{"truncated header", []byte{0x00, 0x44, 0x33, 0x22, 0x11}, CMD_SECURITY_COUNT, "header shorter"},
		{"truncated payload", overclaim, CMD_SECURITY_COUNT, "payload shorter"},
		{"command mismatch", buildTDXReply(CMD_SECURITY_LIST, RESP_CONTROL_PLAIN, []byte{0x00, 0x00}), CMD_SECURITY_COUNT, "does not match"},
		{"length mismatch", putUint16(base, 9, 7), CMD_SECURITY_COUNT, "length fields disagree"},
		{"unknown control", putUint16(base, 5, 0x0009), CMD_SECURITY_COUNT, "unknown control"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client, close := pipeWith(tc.frame)
			defer close()
			_, err := readRawResponse(client, tc.wantCmd)
			if err == nil {
				t.Fatalf("expected an error")
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error %q does not contain %q", err.Error(), tc.wantErr)
			}
		})
	}
}

func TestReadRawResponseGarbagePayloadCompressed(t *testing.T) {
	frame := buildTDXReply(CMD_SECURITY_COUNT, RESP_CONTROL_COMPRESSED, []byte{0x00, 0x00, 0x00})
	client, close := pipeWith(frame)
	defer close()

	_, err := readRawResponse(client, CMD_SECURITY_COUNT)
	if err == nil {
		t.Fatal("expected a decompression error")
	}
	if !strings.Contains(err.Error(), "decompressing") {
		t.Errorf("error %q does not name decompression", err.Error())
	}
}

// putUint16 returns frame with a little-endian uint16 written at offset.
func putUint16(frame []byte, offset int, v uint16) []byte {
	out := append([]byte(nil), frame...)
	binary.LittleEndian.PutUint16(out[offset:], v)
	return out
}

func TestZlibDecompressRoundTrip(t *testing.T) {
	payload := []byte{0x2a, 0x00, 0x00, 0x00}
	var buf bytes.Buffer
	zw := zlib.NewWriter(&buf)
	if _, err := zw.Write(payload); err != nil {
		t.Fatalf("compress: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	got, err := zlibDecompress(buf.Bytes())
	if err != nil {
		t.Fatalf("zlibDecompress: %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Errorf("got %v, want %v", got, payload)
	}

	// A raw deflate stream has no zlib header and must be rejected, so a
	// compressed reply cannot be silently misread as a plain one.
	var raw bytes.Buffer
	dw, err := flate.NewWriter(&raw, flate.NoCompression)
	if err != nil {
		t.Fatalf("flate: %v", err)
	}
	dw.Write(payload)
	dw.Close()
	if _, err := zlibDecompress(raw.Bytes()); err == nil {
		t.Error("zlibDecompress accepted a raw deflate stream")
	}
}

func TestNewTDXAdapter(t *testing.T) {
	a := NewDefaultTDXAdapter()
	if a == nil {
		t.Fatal("Adapter is nil")
	}
	if a.Name() != "tdx" {
		t.Errorf("Name: got %s, want tdx", a.Name())
	}
}

func TestNewTDXAdapterWithHosts(t *testing.T) {
	a := NewTDXAdapter([]string{"127.0.0.1:7709"})
	if a == nil {
		t.Fatal("Adapter is nil")
	}
	if len(a.hosts) != 1 {
		t.Errorf("Hosts: got %d, want 1", len(a.hosts))
	}
}

func TestTDXRequestUnknownInterface(t *testing.T) {
	a := NewDefaultTDXAdapter()
	_, err := a.Request(context.Background(), map[string]interface{}{
		"interface": "unknown_interface",
	})
	if err == nil {
		t.Fatal("Expected error for unknown interface")
	}
}

func TestTDXRequestNoInterface(t *testing.T) {
	a := NewDefaultTDXAdapter()
	_, err := a.Request(context.Background(), map[string]interface{}{})
	if err == nil {
		t.Fatal("Expected error for missing interface")
	}
}

func TestCommandFromString(t *testing.T) {
	tests := []struct {
		name string
		want uint16
	}{
		{"kline", CMD_KLINES},
		{"security_list", CMD_SECURITY_LIST},
		{"category_quotes", CMD_CATEGORY_QUOTES},
		{"auction_process", CMD_AUCTION_PROCESS},
		{"stock_kline_daily_tdx", CMD_KLINES},
	}

	for _, tc := range tests {
		cmd, err := commandFromString(tc.name)
		if err != nil {
			t.Fatalf("commandFromString(%q): %v", tc.name, err)
		}
		if cmd != tc.want {
			t.Errorf("commandFromString(%q): got 0x%x, want 0x%x", tc.name, cmd, tc.want)
		}
	}

	_, err := commandFromString("unknown")
	if err == nil {
		t.Fatal("commandFromString(unknown): expected error")
	}
}

func TestPeriodPairForInterface(t *testing.T) {
	tests := []struct {
		iface string
		want  PeriodPair
	}{
		{"kline_daily", PeriodPair{PERIOD_DAILY, 1}},
		{"stock_kline_daily_tdx", PeriodPair{PERIOD_DAILY, 1}},
	}

	for _, tc := range tests {
		result := periodPairForInterface(tc.iface, map[string]interface{}{})
		if result != tc.want {
			t.Errorf("periodPairForInterface(%q): got %+v, want %+v", tc.iface, result, tc.want)
		}
	}
}

func TestMarketFromString(t *testing.T) {
	tests := []struct {
		s    string
		want uint8
	}{
		{"0", MARKET_SZ},
		{"1", MARKET_SH},
		{"2", MARKET_BJ},
	}

	for _, tc := range tests {
		result := marketFromString(tc.s)
		if result != tc.want {
			t.Errorf("marketFromString(%q): got %d, want %d", tc.s, result, tc.want)
		}
	}
}

func TestMarketToCode(t *testing.T) {
	tests := []struct {
		market int
		want   string
	}{
		{0, "sz"},
		{1, "sh"},
		{2, "bj"},
	}

	for _, tc := range tests {
		result := marketToCode(tc.market)
		if result != tc.want {
			t.Errorf("marketToCode(%d): got %s, want %s", tc.market, result, tc.want)
		}
	}
}

func TestMarketToExchange(t *testing.T) {
	tests := []struct {
		market int
		want   string
	}{
		{0, "SZSE"},
		{1, "SSE"},
		{2, "BSE"},
	}

	for _, tc := range tests {
		result := marketToExchange(tc.market)
		if result != tc.want {
			t.Errorf("marketToExchange(%d): got %s, want %s", tc.market, result, tc.want)
		}
	}
}

func TestSplitCSV(t *testing.T) {
	tests := []struct {
		input string
		want  []string
	}{
		{"000001,000002", []string{"000001", "000002"}},
		{"000001", []string{"000001"}},
	}

	for _, tc := range tests {
		result := splitCSV(tc.input)
		if len(result) != len(tc.want) {
			t.Errorf("splitCSV(%q): got %d parts, want %d", tc.input, len(result), len(tc.want))
			continue
		}
		for i, s := range result {
			if s != tc.want[i] {
				t.Errorf("splitCSV(%q)[%d]: got %s, want %s", tc.input, i, s, tc.want[i])
			}
		}
	}
}

func TestStrval(t *testing.T) {
	params := map[string]interface{}{
		"key1": "value1",
	}
	if strval(params, "key1", "default") != "value1" {
		t.Errorf("strval: got unexpected value")
	}
	if strval(params, "missing", "default") != "default" {
		t.Errorf("strval: got unexpected default")
	}
}

func TestIntval(t *testing.T) {
	params := map[string]interface{}{
		"key1": 42,
		"str1": "7",
	}
	if intval(params, "key1", 0) != 42 {
		t.Errorf("intval(int): got unexpected value")
	}
	if intval(params, "str1", 0) != 7 {
		t.Errorf("intval(string): got unexpected value")
	}
	if intval(params, "missing", 99) != 99 {
		t.Errorf("intval(missing): got unexpected value")
	}
}

func TestBuildLimitLadderRequest(t *testing.T) {
	req, err := buildLimitLadderRequest(map[string]interface{}{
		"stock_code": "000001.SZ",
	})
	if err != nil {
		t.Fatalf("buildLimitLadderRequest failed: %v", err)
	}
	if req == nil {
		t.Fatal("Request is nil")
	}
}

func TestBuildThemeStrengthRequest(t *testing.T) {
	req, err := buildThemeStrengthRequest(map[string]interface{}{})
	if err != nil {
		t.Fatalf("buildThemeStrengthRequest failed: %v", err)
	}
	if req == nil {
		t.Fatal("Request is nil")
	}
}

func TestBuildQuotesRequest(t *testing.T) {
	req, err := buildQuotesRequest(map[string]interface{}{
		"market": "sz",
		"code":   "000001",
	}, false)
	if err != nil {
		t.Fatalf("buildQuotesRequest failed: %v", err)
	}
	if req == nil {
		t.Fatal("Request is nil")
	}
}

func TestBuildSecurityListRequest(t *testing.T) {
	req, err := buildSecurityListRequest(map[string]interface{}{})
	if err != nil {
		t.Fatalf("buildSecurityListRequest failed: %v", err)
	}
	if req == nil {
		t.Fatal("Request is nil")
	}
}

func TestBuildCategoryQuotesRequest(t *testing.T) {
	req, err := buildCategoryQuotesRequest(map[string]interface{}{
		"sort": "5",
	})
	if err != nil {
		t.Fatalf("buildCategoryQuotesRequest failed: %v", err)
	}
	if req == nil {
		t.Fatal("Request is nil")
	}
}

func TestEncodeRequest(t *testing.T) {
	req := &WireRequest{
		Command: CMD_HEARTBEAT,
		Payload: []byte("test"),
	}
	data, err := encodeRequest(req)
	if err != nil {
		t.Fatalf("encodeRequest failed: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("Encoded data is empty")
	}
}

func TestParseSecurityCountRows(t *testing.T) {
	resp := &WireResponse{
		Command: CMD_SECURITY_COUNT,
		MsgID:   1,
		Data:    []byte{5, 0}, // count = 5 in little-endian
	}
	result, err := parseSecurityCountRows(resp)
	if err != nil {
		t.Fatalf("parseSecurityCountRows failed: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("Expected 1 result, got %d", len(result))
	}
	if result[0]["count"] != 5 {
		t.Errorf("count: got %v, want 5", result[0]["count"])
	}
}

func TestCompactFloat(t *testing.T) {
	result := compactFloat(0)
	if result != 0.0 {
		t.Errorf("compactFloat(0): got %v, want 0.0", result)
	}
}

func TestGBK2Str(t *testing.T) {
	result := gbk2str([]byte{0x00, 0x00})
	if result != "" {
		t.Errorf("gbk2str: got %q, want empty", result)
	}
}

func TestAscii2Str(t *testing.T) {
	result := ascii2str([]byte("test"))
	if result != "test" {
		t.Errorf("ascii2str: got %q, want test", result)
	}
}

func TestDeadlineFromContext(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	result := deadlineFromContext(ctx, 3)
	if result.IsZero() {
		t.Fatal("deadlineFromContext returned zero time")
	}
}
