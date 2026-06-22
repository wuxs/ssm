package tui

import (
	"net"
	"testing"
	"time"
)

func TestCheckTCPStatusReportsOnlineForListeningPort(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer listener.Close()

	status := checkTCPStatus(listener.Addr().String(), 200*time.Millisecond)

	if status != StatusOnline {
		t.Fatalf("status = %q, want %q", status, StatusOnline)
	}
}

func TestCheckTCPStatusReportsOfflineForClosedPort(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := listener.Addr().String()
	listener.Close()

	status := checkTCPStatus(addr, 200*time.Millisecond)

	if status != StatusOffline {
		t.Fatalf("status = %q, want %q", status, StatusOffline)
	}
}

func TestStatusCheckCmdMarksRowsCheckingThenReportsResults(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer listener.Close()

	rows := []ConnectionRow{{Key: "deploy@" + listener.Addr().String(), Address: listener.Addr().String()}}

	cmd := checkStatusesCmd(rows, 2, 200*time.Millisecond)
	msg := cmd().(statusBatchMsg)

	if got := msg.Results[rows[0].Key]; got != StatusOnline {
		t.Fatalf("result = %q, want %q", got, StatusOnline)
	}
}
