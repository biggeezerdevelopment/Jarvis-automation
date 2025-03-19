package monitoring

import (
	"encoding/json"
	"fmt"
	"runtime"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/load"
)

// SystemMetrics represents system metrics data
type SystemMetrics struct {
	Timestamp  time.Time `json:"timestamp"`
	CPUUsage   []float64 `json:"cpu_usage"`   // Per CPU usage percentage
	LoadAvg1   float64   `json:"load_avg_1"`  // 1 minute load average
	LoadAvg5   float64   `json:"load_avg_5"`  // 5 minute load average
	LoadAvg15  float64   `json:"load_avg_15"` // 15 minute load average
	NumCPU     int       `json:"num_cpu"`     // Number of CPUs
	SystemType string    `json:"system_type"` // Operating system type
}

// Monitor handles system monitoring operations
type Monitor struct {
	interval time.Duration
}

// NewMonitor creates a new system monitor
func NewMonitor(interval time.Duration) *Monitor {
	return &Monitor{
		interval: interval,
	}
}

// GetMetrics gathers current system metrics
func (m *Monitor) GetMetrics() (*SystemMetrics, error) {
	metrics := &SystemMetrics{
		Timestamp:  time.Now(),
		NumCPU:     runtime.NumCPU(),
		SystemType: runtime.GOOS,
	}

	// Get CPU usage percentages
	cpuPercentages, err := cpu.Percent(m.interval, true) // true for per-CPU usage
	if err != nil {
		return nil, fmt.Errorf("failed to get CPU usage: %v", err)
	}
	metrics.CPUUsage = cpuPercentages

	// Get load averages (works on Unix-like systems)
	if runtime.GOOS != "windows" {
		loadAvg, err := load.Avg()
		if err != nil {
			return nil, fmt.Errorf("failed to get load averages: %v", err)
		}
		metrics.LoadAvg1 = loadAvg.Load1
		metrics.LoadAvg5 = loadAvg.Load5
		metrics.LoadAvg15 = loadAvg.Load15
	} else {
		// For Windows, we'll use CPU usage as a proxy for load average
		avgCPU := 0.0
		for _, usage := range cpuPercentages {
			avgCPU += usage
		}
		avgCPU /= float64(len(cpuPercentages))
		metrics.LoadAvg1 = avgCPU
		metrics.LoadAvg5 = avgCPU
		metrics.LoadAvg15 = avgCPU
	}

	return metrics, nil
}

// ToJSON converts metrics to JSON
func (m *SystemMetrics) ToJSON() ([]byte, error) {
	return json.Marshal(m)
}

// String returns a human-readable representation of the metrics
func (m *SystemMetrics) String() string {
	cpuStr := "CPU Usage:"
	for i, usage := range m.CPUUsage {
		cpuStr += fmt.Sprintf(" CPU%d: %.2f%%", i, usage)
	}

	return fmt.Sprintf(
		"System Metrics [%s]\n"+
			"System Type: %s\n"+
			"Number of CPUs: %d\n"+
			"%s\n"+
			"Load Averages: 1min: %.2f, 5min: %.2f, 15min: %.2f",
		m.Timestamp.Format(time.RFC3339),
		m.SystemType,
		m.NumCPU,
		cpuStr,
		m.LoadAvg1,
		m.LoadAvg5,
		m.LoadAvg15,
	)
}
