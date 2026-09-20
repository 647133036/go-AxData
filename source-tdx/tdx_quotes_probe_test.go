package tdx

import (
	"context"
	"encoding/binary"
	"testing"
	"time"
)

// TestProbe_QuoteCommandsNotServed documents the live state of the multi-
// symbol quote commands. pytdx's reference implementation (command 0x053E with
// its 20-byte GetSecurityQuotesCmd header) returns zero rows against every
// reachable 7709 server, so parseQuotesRows/parseCategoryQuoteRows are only
// exercised by fixtures today. Kept as a probe because the result changes the
// risk profile of those parsers: a silent empty reply, not a malformed one.
func TestProbe_QuoteCommandsNotServed(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping live test in short mode")
	}
	ctx := context.Background()
	a := NewDefaultTDXAdapter()
	host := a.Hosts()[0]

	codes := []string{"000001", "000002", "000003"}

	header := make([]byte, 10)
	binary.LittleEndian.PutUint16(header[0:], 0x0005)
	binary.LittleEndian.PutUint16(header[8:], uint16(len(codes)))
	body := make([]byte, 0, 10+7*len(codes))
	body = append(body, header...)
	for _, cd := range codes {
		body = append(body, byte(0))
		for i := 0; i < 6; i++ {
			if i < len(cd) {
				body = append(body, cd[i])
			} else {
				body = append(body, 0)
			}
		}
	}
	catReq, _ := buildCategoryQuotesRequest(map[string]interface{}{"market": "sz"})

	for _, c := range []struct {
		name  string
		cmd   uint16
		body  []byte
		parse func(*WireResponse) int
	}{
		{"explicit_quotes 0x054C", CMD_EXPLICIT_QUOTES, body, func(r *WireResponse) int {
			rows, _ := parseQuotesRows(r, true)
			return len(rows)
		}},
		{"legacy_quotes 0x053E", CMD_LEGACY_QUOTES, body, func(r *WireResponse) int {
			rows, _ := parseQuotesRows(r, false)
			return len(rows)
		}},
		{"category_quotes 0x054B", catReq.Command, catReq.Payload, func(r *WireResponse) int {
			rows, _ := parseCategoryQuoteRows(r)
			return len(rows)
		}},
	} {
		conn, err := dialWithContext(ctx, host)
		if err != nil {
			t.Logf("%s: dial: %v", c.name, err)
			continue
		}
		if err := a.doSetup(conn); err != nil {
			t.Logf("%s: setup: %v", c.name, err)
			conn.Close()
			continue
		}
		conn.SetDeadline(time.Now().Add(5 * time.Second))
		resp, err := a.exchange(conn, &WireRequest{Command: c.cmd, Payload: c.body})
		conn.Close()
		if err != nil {
			t.Logf("%s: no reply: %v", c.name, err)
			continue
		}
		rows := c.parse(resp)
		t.Logf("%s: reply=%d bytes rows=%d", c.name, len(resp.Data), rows)
	}
}
