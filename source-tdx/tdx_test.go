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

// buildTDXReply encodes a 7709 reply frame (16-byte header + plain body).
func buildTDXReply(command uint16, data []byte) []byte {
	buf := make([]byte, int(RES_HEADER_SIZE)+len(data))
	binary.LittleEndian.PutUint32(buf[0:], 0x11223344)         // cookie
	binary.LittleEndian.PutUint32(buf[4:], 0x11223344)         // seq echo
	binary.LittleEndian.PutUint16(buf[8:], 0)                  // status = 0
	binary.LittleEndian.PutUint16(buf[10:], command)           // cmd echo
	binary.LittleEndian.PutUint16(buf[12:], uint16(len(data))) // zipsize
	binary.LittleEndian.PutUint16(buf[14:], uint16(len(data))) // unzipsize (= zipsize → plain)
	copy(buf[RES_HEADER_SIZE:], data)
	return buf
}

// buildTDXReplyCompressed encodes a 7709 reply frame with a zlib-compressed
// body (zipsize != unzipsize).
func buildTDXReplyCompressed(command uint16, data []byte) []byte {
	var buf2 bytes.Buffer
	zw := zlib.NewWriter(&buf2)
	zw.Write(data)
	zw.Close()
	body := buf2.Bytes()
	buf := make([]byte, int(RES_HEADER_SIZE)+len(body))
	binary.LittleEndian.PutUint32(buf[0:], 0x11223344)
	binary.LittleEndian.PutUint32(buf[4:], 0x11223344)
	binary.LittleEndian.PutUint16(buf[8:], 0)
	binary.LittleEndian.PutUint16(buf[10:], command)
	binary.LittleEndian.PutUint16(buf[12:], uint16(len(body))) // zipsize (compressed)
	binary.LittleEndian.PutUint16(buf[14:], uint16(len(data))) // unzipsize (original)
	copy(buf[RES_HEADER_SIZE:], body)
	return buf
}

// drainSetupFrame reads one 12-byte request frame (header + payload) from the
// fake server side, mirroring what a real TDX server does during setup.
func drainSetupFrame(conn net.Conn) error {
	head := make([]byte, REQ_HEADER_SIZE)
	if _, err := io.ReadFull(conn, head); err != nil {
		return err
	}
	payloadLen := int(binary.LittleEndian.Uint16(head[6:8])) - 2
	if payloadLen > 0 {
		payload := make([]byte, payloadLen)
		if _, err := io.ReadFull(conn, payload); err != nil {
			return err
		}
	}
	return nil
}

// TestTDXRequestRoundTripAgainstLocalServer drives a real Request() through a
// local server that speaks the wire protocol, proving the 3-frame setup, the
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
		// Drain 3 setup frames, reply to each with an empty 16-byte response.
		for i := 0; i < 3; i++ {
			if err := drainSetupFrame(conn); err != nil {
				return
			}
			conn.Write(buildTDXReply(0, []byte{}))
		}
		// Read the actual command frame.
		head := make([]byte, REQ_HEADER_SIZE)
		if _, err := io.ReadFull(conn, head); err != nil {
			return
		}
		cmd := binary.LittleEndian.Uint16(head[10:12])
		payloadLen := int(binary.LittleEndian.Uint16(head[6:8])) - 2
		if payloadLen > 0 {
			payload := make([]byte, payloadLen)
			if _, err := io.ReadFull(conn, payload); err != nil {
				return
			}
		}
		seen <- cmd
		body := make([]byte, 2)
		binary.LittleEndian.PutUint16(body, want)
		conn.Write(buildTDXReply(CMD_SECURITY_COUNT, body))
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

	if got := <-seen; got != CMD_SECURITY_COUNT {
		t.Errorf("command: got 0x%04X, want 0x%04X", got, CMD_SECURITY_COUNT)
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
	if !strings.Contains(err.Error(), "response header") {
		t.Errorf("error %q does not name the read path", err.Error())
	}
	if elapsed > 5*time.Second {
		t.Errorf("failed after %v; expected a fast failure", elapsed)
	}
}

// startFakeCountServer speaks the 7709 setup + security_count protocol and
// replies with want as the count. The listener is closed when t ends.
func startFakeCountServer(t *testing.T, want uint16) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(conn net.Conn) {
				defer conn.Close()
				for i := 0; i < 3; i++ {
					if err := drainSetupFrame(conn); err != nil {
						return
					}
					conn.Write(buildTDXReply(0, []byte{}))
				}
				head := make([]byte, REQ_HEADER_SIZE)
				if _, err := io.ReadFull(conn, head); err != nil {
					return
				}
				payloadLen := int(binary.LittleEndian.Uint16(head[6:8])) - 2
				if payloadLen > 0 {
					payload := make([]byte, payloadLen)
					if _, err := io.ReadFull(conn, payload); err != nil {
						return
					}
				}
				body := make([]byte, 2)
				binary.LittleEndian.PutUint16(body, want)
				conn.Write(buildTDXReply(CMD_SECURITY_COUNT, body))
			}(conn)
		}
	}()
	return ln.Addr().String()
}

// TestTDXRequestFallsOverToSecondHost: the first host refuses, the second
// speaks the protocol, Request returns the second host's data.
func TestTDXRequestFallsOverToSecondHost(t *testing.T) {
	good := startFakeCountServer(t, 42)
	a := NewTDXAdapter([]string{"127.0.0.1:1", good})
	a.timeout = 1
	a.maxDuration = 5

	rows, err := a.Request(context.Background(), map[string]interface{}{
		"interface": "security_count",
		"market":    "sz",
	})
	if err != nil {
		t.Fatalf("Request: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows: got %d, want 1", len(rows))
	}
	if got := rows[0]["count"].(int); got != 42 {
		t.Fatalf("count: got %d, want 42", got)
	}
}

// TestTDXRequestAllHostsFail: every host is unreachable; Request returns
// after collecting every failure, not after the first one.
func TestTDXRequestAllHostsFail(t *testing.T) {
	a := NewTDXAdapter([]string{"127.0.0.1:1", "127.0.0.1:2"})
	a.timeout = 1
	a.maxDuration = 5

	_, err := a.Request(context.Background(), map[string]interface{}{
		"interface": "security_count",
	})
	if err == nil {
		t.Fatal("expected an error when every host fails")
	}
	if !strings.Contains(err.Error(), "all 2 TDX servers failed") {
		t.Errorf("error %q does not name the host count", err.Error())
	}
}

// TestTDXRequestFirstSuccessWins: two good hosts; Request returns as soon as
// either answers, without waiting for the slower one.
func TestTDXRequestFirstSuccessWins(t *testing.T) {
	fast := startFakeCountServer(t, 7)
	slow := startFakeCountServer(t, 99)
	a := NewTDXAdapter([]string{fast, slow})
	a.timeout = 2
	a.maxDuration = 5

	rows, err := a.Request(context.Background(), map[string]interface{}{
		"interface": "security_count",
		"market":    "sz",
	})
	if err != nil {
		t.Fatalf("Request: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows: got %d, want 1", len(rows))
	}
	got := rows[0]["count"].(int)
	if got != 7 && got != 99 {
		t.Fatalf("count: got %d, want 7 or 99", got)
	}
}

// TestEncodeRequestLengthMatchesFrame pins the frame-length invariant. The two
// length fields (len1 and len2 at offsets [6:8] and [8:10]) both equal
// payload_len+2; a conforming server reads payload_len bytes after the
// 12-byte header.
func TestEncodeRequestLengthMatchesFrame(t *testing.T) {
	frame, err := encodeRequest(&WireRequest{Command: CMD_SECURITY_COUNT, Payload: []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00}})
	if err != nil {
		t.Fatalf("encodeRequest: %v", err)
	}
	if len(frame) != int(REQ_HEADER_SIZE)+6 {
		t.Fatalf("frame len: got %d, want %d", len(frame), int(REQ_HEADER_SIZE)+6)
	}
	len1 := int(binary.LittleEndian.Uint16(frame[6:8]))
	len2 := int(binary.LittleEndian.Uint16(frame[8:10]))
	if len1 != 8 {
		t.Errorf("len1: got %d, want 8", len1)
	}
	if len2 != 8 {
		t.Errorf("len2: got %d, want 8", len2)
	}
	if len1 != len(frame)-int(REQ_HEADER_SIZE)+2 {
		t.Errorf("len1 %d does not match payload+2", len1)
	}
	cmd := binary.LittleEndian.Uint16(frame[10:12])
	if cmd != CMD_SECURITY_COUNT {
		t.Errorf("cmd: got 0x%04X, want 0x%04X", cmd, CMD_SECURITY_COUNT)
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
	frame := buildTDXReply(CMD_SECURITY_COUNT, payload)

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
	frame := buildTDXReplyCompressed(CMD_SECURITY_COUNT, payload)

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
	base := buildTDXReply(CMD_SECURITY_COUNT, []byte{0x01, 0x00})
	overclaim := putUint16(base, 12, 4) // zipsize at [12:14] declares 4 bytes, only 2 follow

	cases := []struct {
		name    string
		frame   []byte
		wantCmd uint16
		wantErr string
	}{
		{"truncated header", []byte{0x00, 0x44, 0x33, 0x22, 0x11}, CMD_SECURITY_COUNT, "header shorter"},
		{"truncated payload", overclaim, CMD_SECURITY_COUNT, "payload shorter"},
		{"command mismatch", buildTDXReply(CMD_SECURITY_LIST, []byte{0x00, 0x00}), CMD_SECURITY_COUNT, "does not match"},
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
	// Build a 16-byte-header response where zipsize != unzipsize but the body
	// is not a valid zlib stream.
	garbage := []byte{0x00, 0x00, 0x00}
	buf := make([]byte, int(RES_HEADER_SIZE)+len(garbage))
	binary.LittleEndian.PutUint32(buf[0:], 0x11223344)
	binary.LittleEndian.PutUint32(buf[4:], 0x11223344)
	binary.LittleEndian.PutUint16(buf[8:], 0)
	binary.LittleEndian.PutUint16(buf[10:], CMD_SECURITY_COUNT)
	binary.LittleEndian.PutUint16(buf[12:], uint16(len(garbage))) // zipsize = 3
	binary.LittleEndian.PutUint16(buf[14:], 100)                  // unzipsize = 100 (≠ zipsize → decompress)
	copy(buf[RES_HEADER_SIZE:], garbage)

	client, close := pipeWith(buf)
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
		"f64":  float64(800),
	}
	if intval(params, "key1", 0) != 42 {
		t.Errorf("intval(int): got unexpected value")
	}
	if intval(params, "str1", 0) != 7 {
		t.Errorf("intval(string): got unexpected value")
	}
	if intval(params, "f64", 0) != 800 {
		t.Errorf("intval(float64): got unexpected value")
	}
	if intval(params, "missing", 99) != 99 {
		t.Errorf("intval(missing): got unexpected default")
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

func encodeKlineBar(timeRaw uint32, openD, closeD, highD, lowD byte, vol, amount uint32, index bool, up, down uint16) []byte {
	buf := make([]byte, 0, 20)
	tmp := make([]byte, 4)
	binary.LittleEndian.PutUint32(tmp, timeRaw)
	buf = append(buf, tmp...)
	buf = append(buf, openD, closeD, highD, lowD)
	binary.LittleEndian.PutUint32(tmp, vol)
	buf = append(buf, tmp...)
	binary.LittleEndian.PutUint32(tmp, amount)
	buf = append(buf, tmp...)
	if index {
		u := make([]byte, 4)
		binary.LittleEndian.PutUint16(u[0:], up)
		binary.LittleEndian.PutUint16(u[2:], down)
		buf = append(buf, u...)
	}
	return buf
}

func TestParseKlineRowsStockDoesNotSkipBreadth(t *testing.T) {
	bar1 := encodeKlineBar(100, 10, 5, 8, 2, 111, 222, false, 0, 0)
	bar2 := encodeKlineBar(200, 1, 2, 3, 0, 333, 444, false, 0, 0)
	body := make([]byte, 2, 2+len(bar1)+len(bar2))
	binary.LittleEndian.PutUint16(body, 2)
	body = append(body, bar1...)
	body = append(body, bar2...)

	rows, err := parseKlineRows(&WireResponse{Data: body}, false)
	if err != nil {
		t.Fatalf("parseKlineRows: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2", len(rows))
	}
	if rows[0]["trade_date"] != uint32(100) {
		t.Errorf("bar1 trade_date: got %v want 100", rows[0]["trade_date"])
	}
	if rows[0]["volume"] != 111 {
		t.Errorf("bar1 volume: got %v want 111", rows[0]["volume"])
	}
	if rows[1]["trade_date"] != uint32(200) {
		t.Errorf("bar2 trade_date: got %v want 200", rows[1]["trade_date"])
	}
	if rows[1]["volume"] != 333 {
		t.Errorf("bar2 volume: got %v want 333", rows[1]["volume"])
	}
}

func TestParseKlineRowsIndexReadsBreadth(t *testing.T) {
	bar := encodeKlineBar(100, 10, 5, 8, 2, 111, 222, true, 1200, 800)
	body := make([]byte, 2, 2+len(bar))
	binary.LittleEndian.PutUint16(body, 1)
	body = append(body, bar...)

	rows, err := parseKlineRows(&WireResponse{Data: body}, true)
	if err != nil {
		t.Fatalf("parseKlineRows: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1", len(rows))
	}
	if rows[0]["up_count"] != 1200 {
		t.Errorf("up_count: got %v want 1200", rows[0]["up_count"])
	}
	if rows[0]["down_count"] != 800 {
		t.Errorf("down_count: got %v want 800", rows[0]["down_count"])
	}
}

func TestParsePriceLimitsRows(t *testing.T) {
	rec := make([]byte, 15)
	rec[0] = 0
	copy(rec[1:7], []byte("000001"))
	binary.LittleEndian.PutUint32(rec[7:11], 1100)
	binary.LittleEndian.PutUint32(rec[11:15], 900)
	body := make([]byte, 2+15)
	binary.LittleEndian.PutUint16(body[:2], 1)
	copy(body[2:], rec)

	rows, err := parsePriceLimitsRows(&WireResponse{Data: body})
	if err != nil {
		t.Fatalf("parsePriceLimitsRows: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1", len(rows))
	}
	if rows[0]["symbol"] != "000001" {
		t.Errorf("symbol: got %v want 000001", rows[0]["symbol"])
	}
	if rows[0]["upper_limit"] != uint32(1100) {
		t.Errorf("upper_limit: got %v want 1100", rows[0]["upper_limit"])
	}
	if rows[0]["lower_limit"] != uint32(900) {
		t.Errorf("lower_limit: got %v want 900", rows[0]["lower_limit"])
	}
}

// putVarint8 encodes a signed value as a single-byte TDX varint: bits 0-5 hold
// the magnitude, bit 6 the sign. Values outside [-63, 63] are not representable.
func putVarint8(v int64) byte {
	m := v
	if m < 0 {
		m = -m
	}
	b := byte(m & 0x3F)
	if v < 0 {
		b |= 0x40
	}
	return b
}

func putVarintFull(v int64) []byte {
	m := v
	neg := false
	if m < 0 {
		m = -m
		neg = true
	}
	first := byte(m & 0x3F)
	if neg {
		first |= 0x40
	}
	rest := m >> 6
	if rest == 0 {
		return []byte{first}
	}
	buf := []byte{first | 0x80}
	for {
		b := byte(rest & 0x7F)
		rest >>= 7
		if rest > 0 {
			b |= 0x80
		}
		buf = append(buf, b)
		if rest == 0 {
			break
		}
	}
	return buf
}

func TestVarintRoundTrip(t *testing.T) {
	for _, v := range []int64{-63, -1, 0, 1, 63, 64, 1000, -1000, 16384, -16384, 409600} {
		buf := putVarintFull(v)
		got, next := varint(buf, 0)
		if got != v {
			t.Errorf("varint(%x): got %d want %d", buf, got, v)
		}
		if next != len(buf) {
			t.Errorf("varint(%x): pos %d want %d", buf, next, len(buf))
		}
	}
	if got, _ := varint([]byte{0x7F}, 0); got != -63 {
		t.Errorf("varint(0x7F): got %d want -63", got)
	}
	if got, _ := varint([]byte{}, 0); got != 0 {
		t.Errorf("varint(empty): got %d want 0", got)
	}
}

func encodeQuoteRecord(market byte, code string, active1 uint16, totalHand, amountRaw, insideDish int64) []byte {
	buf := make([]byte, 0, 44)
	buf = append(buf, market)
	for i := 0; i < 6; i++ {
		if i < len(code) {
			buf = append(buf, code[i])
		} else {
			buf = append(buf, ' ')
		}
	}
	buf = append(buf, byte(active1&0xFF), byte(active1>>8))
	buf = append(buf, putVarint8(60)) // closeRaw
	buf = append(buf, putVarint8(1))  // preCloseDiff
	buf = append(buf, putVarint8(2))  // openDiff
	buf = append(buf, putVarint8(3))  // highDiff
	buf = append(buf, putVarint8(0))  // lowDiff
	buf = append(buf, putVarint8(0))  // time_raw
	buf = append(buf, putVarint8(0))  // unknown
	buf = append(buf, putVarint8(totalHand))
	buf = append(buf, putVarint8(totalHand/2))
	amount := make([]byte, 4)
	binary.LittleEndian.PutUint32(amount, uint32(amountRaw))
	buf = append(buf, amount...)
	buf = append(buf, putVarint8(insideDish))
	buf = append(buf, putVarint8(insideDish*2))
	for j := 0; j < 5; j++ {
		buf = append(buf, putVarint8(int64(-j-1)))
		buf = append(buf, putVarint8(int64(j+1)))
		buf = append(buf, putVarint8(int64(j+1)))
		buf = append(buf, putVarint8(int64(10+j)))
	}
	return buf
}

func TestParseQuotesRowsAdvancesCursorAcrossSymbols(t *testing.T) {
	rec1 := encodeQuoteRecord(0x00, "000001", 256, 40, 1234, 10)
	rec2 := encodeQuoteRecord(0x01, "600519", 512, 50, 5678, 30)
	body := make([]byte, 4, 4+len(rec1)+len(rec2))
	binary.LittleEndian.PutUint16(body[2:], 2)
	body = append(body, rec1...)
	body = append(body, rec2...)

	rows, err := parseQuotesRows(&WireResponse{Data: body}, false)
	if err != nil {
		t.Fatalf("parseQuotesRows: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2", len(rows))
	}

	first := rows[0]
	if first["symbol"] != "000001" {
		t.Errorf("row1 symbol: got %v want 000001", first["symbol"])
	}
	if first["exchange"] != "SZSE" {
		t.Errorf("row1 exchange: got %v want SZSE", first["exchange"])
	}
	if first["open"] != 0.62 || first["close"] != 0.6 || first["high"] != 0.63 || first["low"] != 0.6 {
		t.Errorf("row1 ohlc: got %v/%v/%v/%v want 0.62/0.6/0.63/0.6", first["open"], first["close"], first["high"], first["low"])
	}
	if first["pre_close"] != 0.61 {
		t.Errorf("row1 pre_close: got %v want 0.61", first["pre_close"])
	}
	if first["total_volume"] != 40 || first["current_volume"] != 20 {
		t.Errorf("row1 volumes: got %v/%v want 40/20", first["total_volume"], first["current_volume"])
	}
	if first["amount_raw"] != 1234 {
		t.Errorf("row1 amount_raw: got %v want 1234", first["amount_raw"])
	}
	if first["inside_dish"] != int64(10) || first["outer_disc"] != int64(20) {
		t.Errorf("row1 dishes: got %v/%v want 10/20", first["inside_dish"], first["outer_disc"])
	}
	if first["bid_vol_sum"] != int64(15) || first["ask_vol_sum"] != int64(60) {
		t.Errorf("row1 bid/ask sums: got %v/%v want 15/60", first["bid_vol_sum"], first["ask_vol_sum"])
	}
	if first["active1"] != 256 {
		t.Errorf("row1 active1: got %v want 256", first["active1"])
	}

	second := rows[1]
	if second["symbol"] != "600519" {
		t.Errorf("row2 symbol: got %v want 600519", second["symbol"])
	}
	if second["exchange"] != "SSE" {
		t.Errorf("row2 exchange: got %v want SSE", second["exchange"])
	}
	if second["tdx_code"] != "sh600519" {
		t.Errorf("row2 tdx_code: got %v want sh600519", second["tdx_code"])
	}
	if second["total_volume"] != 50 {
		t.Errorf("row2 total_volume: got %v want 50", second["total_volume"])
	}
	if second["amount_raw"] != 5678 {
		t.Errorf("row2 amount_raw: got %v want 5678", second["amount_raw"])
	}
	if second["inside_dish"] != int64(30) || second["outer_disc"] != int64(60) {
		t.Errorf("row2 dishes: got %v/%v want 30/60", second["inside_dish"], second["outer_disc"])
	}
	if second["bid_vol_sum"] != int64(15) {
		t.Errorf("row2 bid_vol_sum: got %v want 15", second["bid_vol_sum"])
	}
}

func encodeCategoryQuoteRecord(market byte, code string, active1 uint16, totalHand, amountRaw int64) []byte {
	buf := make([]byte, 0, 84)
	buf = append(buf, market)
	for i := 0; i < 6; i++ {
		if i < len(code) {
			buf = append(buf, code[i])
		} else {
			buf = append(buf, ' ')
		}
	}
	buf = append(buf, byte(active1&0xFF), byte(active1>>8))
	buf = append(buf, putVarint8(60))
	buf = append(buf, putVarint8(1))
	buf = append(buf, putVarint8(2))
	buf = append(buf, putVarint8(3))
	buf = append(buf, putVarint8(0))
	buf = append(buf, putVarint8(0))
	buf = append(buf, putVarint8(0))
	buf = append(buf, putVarint8(totalHand))
	buf = append(buf, putVarint8(totalHand/2))
	amount := make([]byte, 4)
	binary.LittleEndian.PutUint32(amount, uint32(amountRaw))
	buf = append(buf, amount...)
	buf = append(buf, putVarint8(0))
	buf = append(buf, putVarint8(0))
	buf = append(buf, putVarint8(-1))
	buf = append(buf, putVarint8(1))
	buf = append(buf, putVarint8(7))
	buf = append(buf, putVarint8(8))
	buf = append(buf, make([]byte, 56)...)
	return buf
}

func TestParseCategoryQuoteRowsAdvancesCursorAcrossSymbols(t *testing.T) {
	rec1 := encodeCategoryQuoteRecord(0x01, "600519", 777, 40, 4321)
	rec2 := encodeCategoryQuoteRecord(0x00, "000001", 256, 50, 8642)
	body := make([]byte, 4, 4+len(rec1)+len(rec2))
	binary.LittleEndian.PutUint16(body[2:], 2)
	body = append(body, rec1...)
	body = append(body, rec2...)

	rows, err := parseCategoryQuoteRows(&WireResponse{Data: body})
	if err != nil {
		t.Fatalf("parseCategoryQuoteRows: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2", len(rows))
	}

	first := rows[0]
	if first["symbol"] != "600519" {
		t.Errorf("row1 symbol: got %v want 600519", first["symbol"])
	}
	if first["exchange"] != "SSE" {
		t.Errorf("row1 exchange: got %v want SSE", first["exchange"])
	}
	if first["close"] != 0.6 {
		t.Errorf("row1 close: got %v want 0.6", first["close"])
	}
	if first["total_volume"] != 40 || first["current_volume"] != 20 {
		t.Errorf("row1 volumes: got %v/%v want 40/20", first["total_volume"], first["current_volume"])
	}
	if first["amount_raw"] != 4321 {
		t.Errorf("row1 amount_raw: got %v want 4321", first["amount_raw"])
	}
	if first["active1"] != 777 {
		t.Errorf("row1 active1: got %v want 777", first["active1"])
	}
	if first["bid1_price"] != 0.59 || first["bid1_volume"] != 7 {
		t.Errorf("row1 bid1: got %v/%v want 0.59/7", first["bid1_price"], first["bid1_volume"])
	}
	if first["ask1_price"] != 0.61 || first["ask1_volume"] != 8 {
		t.Errorf("row1 ask1: got %v/%v want 0.61/8", first["ask1_price"], first["ask1_volume"])
	}

	second := rows[1]
	if second["symbol"] != "000001" {
		t.Errorf("row2 symbol: got %v want 000001", second["symbol"])
	}
	if second["exchange"] != "SZSE" {
		t.Errorf("row2 exchange: got %v want SZSE", second["exchange"])
	}
	if second["tdx_code"] != "sz000001" {
		t.Errorf("row2 tdx_code: got %v want sz000001", second["tdx_code"])
	}
	if second["total_volume"] != 50 {
		t.Errorf("row2 total_volume: got %v want 50", second["total_volume"])
	}
	if second["amount_raw"] != 8642 {
		t.Errorf("row2 amount_raw: got %v want 8642", second["amount_raw"])
	}
	if second["bid1_volume"] != 7 {
		t.Errorf("row2 bid1_volume: got %v want 7", second["bid1_volume"])
	}
}
