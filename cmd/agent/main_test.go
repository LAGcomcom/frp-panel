package main

import "testing"

func TestParseCPUStat(t *testing.T) {
	total, idle, ok := parseCPUStat("cpu  100 20 30 400 50 10 5 0 0 0\n")
	if !ok || total != 615 || idle != 450 {
		t.Fatalf("total=%d idle=%d ok=%v", total, idle, ok)
	}
}

func TestParseMemInfo(t *testing.T) {
	total, used := parseMemInfo("MemTotal: 1000 kB\nMemAvailable: 250 kB\n")
	if total != 1000*1024 || used != 750*1024 {
		t.Fatalf("total=%d used=%d", total, used)
	}
}

func TestParseNetworkBytes(t *testing.T) {
	data := "Inter-| Receive | Transmit\n lo: 100 0 0 0 0 0 0 0 200 0 0 0 0 0 0 0\n eth0: 300 0 0 0 0 0 0 0 400 0 0 0 0 0 0 0\n ens3: 500 0 0 0 0 0 0 0 600 0 0 0 0 0 0 0\n"
	in, out := parseNetworkBytes(data)
	if in != 800 || out != 1000 {
		t.Fatalf("in=%d out=%d", in, out)
	}
}
