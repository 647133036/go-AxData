package tdx

import (
	"bytes"
	"compress/zlib"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"testing"
	"time"
)

// TestProbe_ExHqStrategies tests multiple read/socket strategies against a
// live 7727 ExHq host to find why the server closes the connection after
// the setup response.
func TestProbe_ExHqStrategies(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping live probe in short mode")
	}

	host := "112.74.214.43:7727"
	setup := exSetupFrame
	cmd := exCountFrame

	strategies := []struct {
		name string
		fn   func(addr string, setup, cmd []byte) string
	}{
		{"ReadFull_thenCmd", strategyReadFullThenCmd},
		{"ReadFull_thenPeek", strategyReadFullThenPeek},
		{"ReadHeader_thenCmdPipeline", strategyReadHeaderThenCmdPipeline},
		{"ReadFull_OneBigRead", strategyOneBigRead},
		{"ReadFull_thenSmallDelay", strategyReadFullThenSmallDelay},
		{"SendCmdBeforeReadBody", strategySendCmdBeforeReadBody},
	}

	for _, s := range strategies {
		t.Run(s.name, func(t *testing.T) {
			result := s.fn(host, setup, cmd)
			t.Logf("result: %s", result)
		})
	}
}

// strategyReadFullThenCmd: read setup header+body with io.ReadFull, then send cmd
func strategyReadFullThenCmd(addr string, setup, cmd []byte) string {
	conn := dialProbe(addr)
	if conn == nil {
		return "dial failed"
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(8 * time.Second))

	if _, err := conn.Write(setup); err != nil {
		return fmt.Sprintf("setup write: %v", err)
	}
	head := make([]byte, 16)
	if _, err := io.ReadFull(conn, head); err != nil {
		return fmt.Sprintf("setup header: %v", err)
	}
	zipSize := int(binary.LittleEndian.Uint16(head[12:14]))
	if zipSize > 0 {
		body := make([]byte, zipSize)
		if _, err := io.ReadFull(conn, body); err != nil {
			return fmt.Sprintf("setup body: %v", err)
		}
	}
	// Now try to send command
	if _, err := conn.Write(cmd); err != nil {
		return fmt.Sprintf("cmd write: %v", err)
	}
	cmdHead := make([]byte, 16)
	n, err := io.ReadFull(conn, cmdHead)
	if err != nil {
		return fmt.Sprintf("cmd response: read %d bytes, err=%v", n, err)
	}
	return fmt.Sprintf("SUCCESS! cmdHead=% x", cmdHead)
}

// strategyReadFullThenPeek: read setup header+body, then try to read 1 more byte
func strategyReadFullThenPeek(addr string, setup, cmd []byte) string {
	conn := dialProbe(addr)
	if conn == nil {
		return "dial failed"
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(8 * time.Second))

	if _, err := conn.Write(setup); err != nil {
		return fmt.Sprintf("setup write: %v", err)
	}
	head := make([]byte, 16)
	if _, err := io.ReadFull(conn, head); err != nil {
		return fmt.Sprintf("setup header: %v", err)
	}
	zipSize := int(binary.LittleEndian.Uint16(head[12:14]))
	unzipSize := int(binary.LittleEndian.Uint16(head[14:16]))
	_ = unzipSize
	if zipSize > 0 {
		body := make([]byte, zipSize)
		if _, err := io.ReadFull(conn, body); err != nil {
			return fmt.Sprintf("setup body: %v", err)
		}
	}
	// Try to peek 1 byte
	one := make([]byte, 1)
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, err := conn.Read(one)
	if err != nil {
		return fmt.Sprintf("peek after setup body: n=%d, err=%v (connection closed?)", n, err)
	}
	return fmt.Sprintf("peek got %d bytes: % x (connection still open!)", n, one[:n])
}

// strategyReadHeaderThenCmdPipeline: read only setup header, skip body, send cmd immediately
func strategyReadHeaderThenCmdPipeline(addr string, setup, cmd []byte) string {
	conn := dialProbe(addr)
	if conn == nil {
		return "dial failed"
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(8 * time.Second))

	if _, err := conn.Write(setup); err != nil {
		return fmt.Sprintf("setup write: %v", err)
	}
	head := make([]byte, 16)
	if _, err := io.ReadFull(conn, head); err != nil {
		return fmt.Sprintf("setup header: %v", err)
	}
	zipSize := int(binary.LittleEndian.Uint16(head[12:14]))
	unzipSize := int(binary.LittleEndian.Uint16(head[14:16]))
	_ = unzipSize

	// Don't read the body — just send the command
	if _, err := conn.Write(cmd); err != nil {
		return fmt.Sprintf("cmd write: %v", err)
	}

	// Read whatever comes back (leftover body + cmd response)
	buf := make([]byte, 4096)
	conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	total := 0
	for {
		n, err := conn.Read(buf[total:])
		total += n
		if err != nil {
			break
		}
		if total >= len(buf) {
			break
		}
	}
	if total == 0 {
		return "no data after cmd write"
	}
	// The first zipSize bytes should be the leftover setup body
	// After that, we should see the cmd response header
	if total <= zipSize {
		return fmt.Sprintf("only got %d bytes (all leftover body?), no cmd response", total)
	}
	// Parse cmd response header starting at offset zipSize
	offset := zipSize
	if offset+16 > total {
		return fmt.Sprintf("not enough bytes for cmd header at offset %d (total=%d)", offset, total)
	}
	cmdHead := buf[offset : offset+16]
	cmdZip := int(binary.LittleEndian.Uint16(cmdHead[12:14]))
	cmdUnzip := int(binary.LittleEndian.Uint16(cmdHead[14:16]))
	cmdEcho := binary.LittleEndian.Uint16(cmdHead[10:12])
	return fmt.Sprintf("zipSize=%d unzipSize=%d cmdEcho=0x%04X cmdZip=%d cmdUnzip=%d total=%d", zipSize, unzipSize, cmdEcho, cmdZip, cmdUnzip, total)
}

// strategyOneBigRead: read as much as possible in one Read call
func strategyOneBigRead(addr string, setup, cmd []byte) string {
	conn := dialProbe(addr)
	if conn == nil {
		return "dial failed"
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(8 * time.Second))

	if _, err := conn.Write(setup); err != nil {
		return fmt.Sprintf("setup write: %v", err)
	}
	// Read everything available in one go
	buf := make([]byte, 8192)
	time.Sleep(200 * time.Millisecond) // give server time to send everything
	n, err := conn.Read(buf)
	if err != nil && err != io.EOF {
		return fmt.Sprintf("big read: n=%d, err=%v", n, err)
	}
	if n < 16 {
		return fmt.Sprintf("big read: only %d bytes", n)
	}
	// Parse header
	head := buf[:16]
	zipSize := int(binary.LittleEndian.Uint16(head[12:14]))
	unzipSize := int(binary.LittleEndian.Uint16(head[14:16]))
	totalExpected := 16 + zipSize
	return fmt.Sprintf("big read: got %d bytes, header says zipSize=%d unzipSize=%d, expected total=%d, extra=%d", n, zipSize, unzipSize, totalExpected, n-totalExpected)
}

// strategyReadFullThenSmallDelay: read setup, small delay, then send cmd
func strategyReadFullThenSmallDelay(addr string, setup, cmd []byte) string {
	conn := dialProbe(addr)
	if conn == nil {
		return "dial failed"
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(10 * time.Second))

	if _, err := conn.Write(setup); err != nil {
		return fmt.Sprintf("setup write: %v", err)
	}
	head := make([]byte, 16)
	if _, err := io.ReadFull(conn, head); err != nil {
		return fmt.Sprintf("setup header: %v", err)
	}
	zipSize := int(binary.LittleEndian.Uint16(head[12:14]))
	if zipSize > 0 {
		body := make([]byte, zipSize)
		if _, err := io.ReadFull(conn, body); err != nil {
			return fmt.Sprintf("setup body: %v", err)
		}
	}
	// Wait a bit to see if the connection closes on its own
	time.Sleep(100 * time.Millisecond)
	// Try a peek read
	one := make([]byte, 1)
	conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	n, err := conn.Read(one)
	if err != nil {
		return fmt.Sprintf("after 100ms delay, peek: n=%d, err=%v (connection closed)", n, err)
	}
	return fmt.Sprintf("after 100ms delay, peek got %d bytes (connection alive!)", n)
}

// strategySendCmdBeforeReadBody: send cmd before reading setup body
func strategySendCmdBeforeReadBody(addr string, setup, cmd []byte) string {
	conn := dialProbe(addr)
	if conn == nil {
		return "dial failed"
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(8 * time.Second))

	// Send setup frame
	if _, err := conn.Write(setup); err != nil {
		return fmt.Sprintf("setup write: %v", err)
	}
	// Read only the setup header
	head := make([]byte, 16)
	if _, err := io.ReadFull(conn, head); err != nil {
		return fmt.Sprintf("setup header: %v", err)
	}
	zipSize := int(binary.LittleEndian.Uint16(head[12:14]))
	// Send cmd BEFORE reading the setup body (pipelining)
	if _, err := conn.Write(cmd); err != nil {
		return fmt.Sprintf("cmd write: %v", err)
	}
	// Now read everything: setup body + cmd response
	buf := make([]byte, 8192)
	conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	total := 0
	for total < len(buf) {
		n, err := conn.Read(buf[total:])
		total += n
		if err != nil {
			break
		}
	}
	if total == 0 {
		return "no data"
	}
	// First zipSize bytes should be setup body
	if total < zipSize+16 {
		return fmt.Sprintf("got %d bytes, need %d (setup body + cmd header)", total, zipSize+16)
	}
	// Parse cmd response header at offset zipSize
	cmdHead := buf[zipSize : zipSize+16]
	cmdEcho := binary.LittleEndian.Uint16(cmdHead[10:12])
	cmdZip := int(binary.LittleEndian.Uint16(cmdHead[12:14]))
	cmdUnzip := int(binary.LittleEndian.Uint16(cmdHead[14:16]))
	// Try to read cmd body
	cmdBodyEnd := zipSize + 16 + cmdZip
	if total < cmdBodyEnd {
		return fmt.Sprintf("cmd header OK (echo=0x%04X zip=%d unzip=%d) but body incomplete (got %d/%d)", cmdEcho, cmdZip, cmdUnzip, total-zipSize-16, cmdZip)
	}
	cmdBody := buf[zipSize+16 : cmdBodyEnd]
	// Decompress if needed
	if cmdZip != cmdUnzip && cmdUnzip > 0 {
		zr, err := zlib.NewReader(bytes.NewReader(cmdBody))
		if err != nil {
			return fmt.Sprintf("cmd decompress: %v", err)
		}
		decomp, err := io.ReadAll(zr)
		zr.Close()
		if err != nil {
			return fmt.Sprintf("cmd decompress read: %v", err)
		}
		// Parse instrument count
		if len(decomp) >= 23 {
			count := binary.LittleEndian.Uint32(decomp[19:23])
			return fmt.Sprintf("SUCCESS! instrument_count=%d (cmdEcho=0x%04X)", count, cmdEcho)
		}
		return fmt.Sprintf("decompressed %d bytes but too short for count", len(decomp))
	}
	return fmt.Sprintf("cmdEcho=0x%04X zip=%d unzip=%d", cmdEcho, cmdZip, cmdUnzip)
}

func dialProbe(addr string) net.Conn {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var d net.Dialer
	d.Timeout = 5 * time.Second
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil
	}
	return conn
}
