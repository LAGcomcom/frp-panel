package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"
)

type Metrics struct {
	Timestamp   int64   `json:"timestamp"`
	CPUUsage    float64 `json:"cpu_usage"`
	MemoryUsed  int64   `json:"memory_used"`
	MemoryTotal int64   `json:"memory_total"`
	DiskUsed    int64   `json:"disk_used"`
	DiskTotal   int64   `json:"disk_total"`
	NetIn       int64   `json:"net_in"`
	NetOut      int64   `json:"net_out"`
	LoadAvg1    float64 `json:"load_avg_1"`
	LoadAvg5    float64 `json:"load_avg_5"`
	LoadAvg15   float64 `json:"load_avg_15"`
	Connections int     `json:"connections"`
}

var (
	prevNetIn  int64
	prevNetOut int64
	prevTime   time.Time
)

func main() {
	panelURL := flag.String("panel", "", "Panel URL (e.g., https://panel.example.com)")
	serverID := flag.String("server", "", "Server ID")
	apiKey := flag.String("key", "", "API Key for authentication")
	interval := flag.Int("interval", 5, "Report interval in seconds")
	flag.Parse()

	if *panelURL == "" || *serverID == "" || *apiKey == "" {
		fmt.Println("Usage: agent -panel <url> -server <id> -key <api_key> [-interval 5]")
		os.Exit(1)
	}

	fmt.Printf("Starting agent: panel=%s server=%s interval=%ds\n", *panelURL, *serverID, *interval)

	// Initialize network counters
	initNetwork()

	ticker := time.NewTicker(time.Duration(*interval) * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		metrics := collectMetrics()
		reportMetrics(*panelURL, *serverID, *apiKey, metrics)
	}
}

func initNetwork() {
	in, out := getNetworkBytes()
	prevNetIn = in
	prevNetOut = out
	prevTime = time.Now()
}

func collectMetrics() *Metrics {
	m := &Metrics{
		Timestamp: time.Now().Unix(),
	}

	// CPU usage
	m.CPUUsage = getCPUUsage()

	// Memory
	m.MemoryTotal, m.MemoryUsed = getMemory()

	// Disk
	m.DiskTotal, m.DiskUsed = getDisk()

	// Network (rate per second)
	currentIn, currentOut := getNetworkBytes()
	elapsed := time.Since(prevTime).Seconds()
	if elapsed > 0 && currentIn >= prevNetIn && currentOut >= prevNetOut {
		m.NetIn = int64(float64(currentIn-prevNetIn) / elapsed)
		m.NetOut = int64(float64(currentOut-prevNetOut) / elapsed)
	}
	prevNetIn = currentIn
	prevNetOut = currentOut
	prevTime = time.Now()

	// Load average
	m.LoadAvg1, m.LoadAvg5, m.LoadAvg15 = getLoadAvg()

	// Connections
	m.Connections = getConnections()

	return m
}

func getCPUUsage() float64 {
	if runtime.GOOS == "linux" {
		first, err := os.ReadFile("/proc/stat")
		if err != nil {
			return 0
		}
		total1, idle1, ok := parseCPUStat(string(first))
		if !ok {
			return 0
		}
		time.Sleep(200 * time.Millisecond)
		second, err := os.ReadFile("/proc/stat")
		if err != nil {
			return 0
		}
		total2, idle2, ok := parseCPUStat(string(second))
		if !ok || total2 <= total1 {
			return 0
		}
		totalDelta := total2 - total1
		idleDelta := idle2 - idle1
		return (1 - float64(idleDelta)/float64(totalDelta)) * 100
	}
	return 0
}

func parseCPUStat(data string) (total, idle uint64, ok bool) {
	line := strings.SplitN(data, "\n", 2)[0]
	fields := strings.Fields(line)
	if len(fields) < 5 || fields[0] != "cpu" {
		return 0, 0, false
	}
	values := make([]uint64, 0, len(fields)-1)
	for _, field := range fields[1:] {
		value, err := strconv.ParseUint(field, 10, 64)
		if err != nil {
			return 0, 0, false
		}
		total += value
		values = append(values, value)
	}
	idle = values[3]
	if len(values) > 4 {
		idle += values[4]
	}
	return total, idle, true
}

func getMemory() (int64, int64) {
	if runtime.GOOS == "linux" {
		data, err := os.ReadFile("/proc/meminfo")
		if err == nil {
			return parseMemInfo(string(data))
		}
	}
	return 0, 0
}

func parseMemInfo(data string) (total, used int64) {
	values := make(map[string]int64)
	for _, line := range strings.Split(data, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		value, err := strconv.ParseInt(fields[1], 10, 64)
		if err == nil {
			values[strings.TrimSuffix(fields[0], ":")] = value * 1024
		}
	}
	total = values["MemTotal"]
	available := values["MemAvailable"]
	if available == 0 {
		available = values["MemFree"] + values["Buffers"] + values["Cached"]
	}
	if total > available {
		used = total - available
	}
	return total, used
}

func getDisk() (int64, int64) {
	if runtime.GOOS == "linux" {
		out, err := exec.Command("sh", "-c", "df -B1 / | tail -1 | awk '{print $2, $3}'").Output()
		if err == nil {
			parts := strings.Fields(string(out))
			if len(parts) >= 2 {
				total, _ := strconv.ParseInt(parts[0], 10, 64)
				used, _ := strconv.ParseInt(parts[1], 10, 64)
				return total, used
			}
		}
	}
	return 0, 0
}

func getNetworkBytes() (int64, int64) {
	if runtime.GOOS == "linux" {
		data, err := os.ReadFile("/proc/net/dev")
		if err == nil {
			return parseNetworkBytes(string(data))
		}
	}
	return 0, 0
}

func parseNetworkBytes(data string) (in, out int64) {
	for _, line := range strings.Split(data, "\n") {
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 || strings.TrimSpace(parts[0]) == "lo" {
			continue
		}
		fields := strings.Fields(parts[1])
		if len(fields) < 9 {
			continue
		}
		received, errIn := strconv.ParseInt(fields[0], 10, 64)
		transmitted, errOut := strconv.ParseInt(fields[8], 10, 64)
		if errIn == nil && errOut == nil {
			in += received
			out += transmitted
		}
	}
	return in, out
}

func getLoadAvg() (float64, float64, float64) {
	if runtime.GOOS == "linux" {
		out, err := os.ReadFile("/proc/loadavg")
		if err == nil {
			parts := strings.Fields(string(out))
			if len(parts) >= 3 {
				l1, _ := strconv.ParseFloat(parts[0], 64)
				l5, _ := strconv.ParseFloat(parts[1], 64)
				l15, _ := strconv.ParseFloat(parts[2], 64)
				return l1, l5, l15
			}
		}
	}
	return 0, 0, 0
}

func getConnections() int {
	if runtime.GOOS == "linux" {
		total := 0
		for _, path := range []string{"/proc/net/tcp", "/proc/net/tcp6", "/proc/net/udp", "/proc/net/udp6"} {
			data, err := os.ReadFile(path)
			if err == nil {
				lines := strings.Split(strings.TrimSpace(string(data)), "\n")
				if len(lines) > 1 {
					total += len(lines) - 1
				}
			}
		}
		return total
	}
	return 0
}

func reportMetrics(panelURL, serverID, apiKey string, metrics *Metrics) {
	data, _ := json.Marshal(metrics)
	url := fmt.Sprintf("%s/api/servers/%s/metrics", panelURL, serverID)

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(data))
	if err != nil {
		fmt.Printf("Error creating request: %v\n", err)
		return
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("Error reporting metrics: %v\n", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		fmt.Printf("Error response: %s %s\n", resp.Status, body)
	}
}
