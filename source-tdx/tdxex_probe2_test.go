package tdx

import (
	"bytes"
	"compress/zlib"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

// TestProbe_ExHqSocketOpts tests the 92-byte setup frame with various socket options.
func TestProbe_ExHqSocketOpts(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping live probe in short mode")
	}

	host := "112.74.214.43:7727"

	strategies := []struct {
		name      string
		noDelay   bool
		keepAlive bool
	}{
		{"DefaultGo", true, true}, // Go defaults: NODELAY on, KEEPALIVE on
		{"NoNodelay_KeepAlive", false, true},
		{"NoNodelay_NoKeepAlive", false, false},
		{"Nodelay_NoKeepAlive", true, false},
	}

	for _, s := range strategies {
		t.Run(s.name, func(t *testing.T) {
			result := probeWithOpts(host, s.noDelay, s.keepAlive)
			t.Logf("result: %s", result)
		})
	}

	// Also test with raw syscall socket (no Go wrappers)
	t.Run("RawSyscallSocket", func(t *testing.T) {
		result := probeWithRawSocket(host)
		t.Logf("result: %s", result)
	})
}

func probeWithOpts(addr string, noDelay, keepAlive bool) string {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var d net.Dialer
	d.Timeout = 5 * time.Second
	d.KeepAlive = -1 // disable keep-alive in dialer
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return fmt.Sprintf("dial: %v", err)
	}
	defer conn.Close()

	if tc, ok := conn.(*net.TCPConn); ok {
		tc.SetNoDelay(noDelay)
		tc.SetKeepAlive(keepAlive)
	}

	conn.SetDeadline(time.Now().Add(8 * time.Second))

	// Send 92-byte setup frame
	if _, err := conn.Write(exSetupFrame); err != nil {
		return fmt.Sprintf("setup write: %v", err)
	}

	// Read setup response
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

	// Send command
	cmd := exCountFrame
	if _, err := conn.Write(cmd); err != nil {
		return fmt.Sprintf("cmd write: %v", err)
	}

	// Read command response
	cmdHead := make([]byte, 16)
	if _, err := io.ReadFull(conn, cmdHead); err != nil {
		return fmt.Sprintf("cmd header: %v", err)
	}
	cmdZip := int(binary.LittleEndian.Uint16(cmdHead[12:14]))
	cmdUnzip := int(binary.LittleEndian.Uint16(cmdHead[14:16]))
	if cmdZip > 0 {
		cmdBody := make([]byte, cmdZip)
		if _, err := io.ReadFull(conn, cmdBody); err != nil {
			return fmt.Sprintf("cmd body: %v", err)
		}
		if cmdZip != cmdUnzip && cmdUnzip > 0 {
			zr, err := zlib.NewReader(bytes.NewReader(cmdBody))
			if err != nil {
				return fmt.Sprintf("decompress: %v", err)
			}
			decomp, _ := io.ReadAll(zr)
			zr.Close()
			if len(decomp) >= 23 {
				count := binary.LittleEndian.Uint32(decomp[19:23])
				return fmt.Sprintf("SUCCESS! instrument_count=%d", count)
			}
			return fmt.Sprintf("decompressed %d bytes (too short)", len(decomp))
		}
	}
	return fmt.Sprintf("SUCCESS! cmdZip=%d", cmdZip)
}

func probeWithRawSocket(addr string) string {
	// Parse address
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Sprintf("split: %v", err)
	}
	portNum := 0
	for _, c := range port {
		portNum = portNum*10 + int(c-'0')
	}

	// Create raw TCP socket (like Python's socket.socket(AF_INET, SOCK_STREAM))
	fd, err := unix.Socket(unix.AF_INET, unix.SOCK_STREAM, 0)
	if err != nil {
		return fmt.Sprintf("socket: %v", err)
	}
	defer unix.Close(fd)

	// Set timeout
	timeout := unix.Timeval{Sec: 8, Usec: 0}
	unix.SetsockoptTimeval(fd, unix.SOL_SOCKET, unix.SO_RCVTIMEO, &timeout)
	unix.SetsockoptTimeval(fd, unix.SOL_SOCKET, unix.SO_SNDTIMEO, &timeout)

	// Connect
	var ip [4]byte
	copy(ip[:], net.ParseIP(host).To4())
	err = unix.Connect(fd, &unix.SockaddrInet4{Port: portNum, Addr: ip})
	if err != nil {
		return fmt.Sprintf("connect: %v", err)
	}

	// Send setup frame
	_, err = unix.Write(fd, exSetupFrame)
	if err != nil {
		return fmt.Sprintf("setup write: %v", err)
	}

	// Read setup response header (16 bytes)
	head := make([]byte, 16)
	n, err := unix.Read(fd, head)
	if err != nil {
		return fmt.Sprintf("setup header read: %v (n=%d)", err, n)
	}
	if n < 16 {
		return fmt.Sprintf("setup header short: %d bytes", n)
	}
	zipSize := int(binary.LittleEndian.Uint16(head[12:14]))

	// Read setup response body
	if zipSize > 0 {
		body := make([]byte, zipSize)
		total := 0
		for total < zipSize {
			n, err := unix.Read(fd, body[total:])
			if err != nil || n == 0 {
				break
			}
			total += n
		}
	}

	// Send command frame
	_, err = unix.Write(fd, exCountFrame)
	if err != nil {
		return fmt.Sprintf("cmd write: %v", err)
	}

	// Read command response
	cmdHead := make([]byte, 16)
	total := 0
	for total < 16 {
		n, err := unix.Read(fd, cmdHead[total:])
		if err != nil || n == 0 {
			return fmt.Sprintf("cmd header read: %v (total=%d)", err, total)
		}
		total += n
	}

	cmdZip := int(binary.LittleEndian.Uint16(cmdHead[12:14]))
	cmdUnzip := int(binary.LittleEndian.Uint16(cmdHead[14:16]))

	if cmdZip > 0 {
		cmdBody := make([]byte, cmdZip)
		total = 0
		for total < cmdZip {
			n, err := unix.Read(fd, cmdBody[total:])
			if err != nil || n == 0 {
				break
			}
			total += n
		}
		if cmdZip != cmdUnzip && cmdUnzip > 0 && total >= cmdZip {
			zr, err := zlib.NewReader(bytes.NewReader(cmdBody))
			if err != nil {
				return fmt.Sprintf("decompress: %v", err)
			}
			decomp, _ := io.ReadAll(zr)
			zr.Close()
			if len(decomp) >= 23 {
				count := binary.LittleEndian.Uint32(decomp[19:23])
				return fmt.Sprintf("SUCCESS! instrument_count=%d", count)
			}
		}
	}
	return fmt.Sprintf("OK cmdZip=%d cmdUnzip=%d", cmdZip, cmdUnzip)
}

// Ensure syscall is used (for older Go versions)
var _ = syscall.Socket
