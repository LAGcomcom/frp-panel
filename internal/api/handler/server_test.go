package handler

import (
	"net"
	"testing"
	"time"
)

func TestMeasureTCPLatencyNeverReturnsZeroForReachableServer(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer listener.Close()

	latency, reachable := measureTCPLatency(listener.Addr().String(), time.Second)
	if !reachable {
		t.Fatal("listener was reported unreachable")
	}
	if latency < 1 {
		t.Fatalf("latency = %d, want at least 1ms", latency)
	}
}
