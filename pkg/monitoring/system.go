package monitoring

import (
	"encoding/json"
	"fmt"
	"runtime"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/load"
	"github.com/shirou/gopsutil/v3/mem"
)

// SystemMetrics represents system metrics data
type SystemMetrics struct {
	Timestamp   time.Time    `json:"timestamp"`
	CPUUsage    []float64    `json:"cpu_usage"`    // Per CPU usage percentage
	LoadAvg1    float64      `json:"load_avg_1"`   // 1 minute load average
	LoadAvg5    float64      `json:"load_avg_5"`   // 5 minute load average
	LoadAvg15   float64      `json:"load_avg_15"`  // 15 minute load average
	NumCPU      int          `json:"num_cpu"`      // Number of CPUs
	SystemType  string       `json:"system_type"`  // Operating system type
	TotalMemory uint64       `json:"total_memory"` // Total physical memory in bytes
	UsedMemory  uint64       `json:"used_memory"`  // Used physical memory in bytes
	FreeMemory  uint64       `json:"free_memory"`  // Free physical memory in bytes
	MemoryUsage float64      `json:"memory_usage"` // Memory usage percentage
	SwapTotal   uint64       `json:"swap_total"`   // Total swap space in bytes
	SwapUsed    uint64       `json:"swap_used"`    // Used swap space in bytes
	SwapFree    uint64       `json:"swap_free"`    // Free swap space in bytes
	SwapUsage   float64      `json:"swap_usage"`   // Swap usage percentage
	DiskMetrics []DiskMetric `json:"disk_metrics"` // Metrics for each disk partition
}

// DiskMetric represents metrics for a single disk partition
type DiskMetric struct {
	Path         string  `json:"path"`          // Mount point path
	Total        uint64  `json:"total"`         // Total size in bytes
	Used         uint64  `json:"used"`          // Used space in bytes
	Free         uint64  `json:"free"`          // Free space in bytes
	UsagePercent float64 `json:"usage_percent"` // Usage percentage
	FSType       string  `json:"fs_type"`       // File system type
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
	cpuPercentages, err := cpu.Percent(m.interval, true)
	if err != nil {
		return nil, fmt.Errorf("failed to get CPU usage: %v", err)
	}
	metrics.CPUUsage = cpuPercentages

	// Get memory statistics
	memStats, err := mem.VirtualMemory()
	if err != nil {
		return nil, fmt.Errorf("failed to get memory stats: %v", err)
	}

	metrics.TotalMemory = memStats.Total
	metrics.UsedMemory = memStats.Used
	metrics.FreeMemory = memStats.Free
	metrics.MemoryUsage = memStats.UsedPercent

	// Get swap statistics
	swapStats, err := mem.SwapMemory()
	if err != nil {
		return nil, fmt.Errorf("failed to get swap stats: %v", err)
	}

	metrics.SwapTotal = swapStats.Total
	metrics.SwapUsed = swapStats.Used
	metrics.SwapFree = swapStats.Free
	metrics.SwapUsage = swapStats.UsedPercent

	// Get disk statistics
	partitions, err := disk.Partitions(false) // false means physical partitions only
	if err != nil {
		return nil, fmt.Errorf("failed to get disk partitions: %v", err)
	}

	metrics.DiskMetrics = make([]DiskMetric, 0, len(partitions))
	for _, partition := range partitions {
		usage, err := disk.Usage(partition.Mountpoint)
		if err != nil {
			continue // Skip this partition if we can't get usage info
		}

		diskMetric := DiskMetric{
			Path:         partition.Mountpoint,
			Total:        usage.Total,
			Used:         usage.Used,
			Free:         usage.Free,
			UsagePercent: usage.UsedPercent,
			FSType:       partition.Fstype,
		}
		metrics.DiskMetrics = append(metrics.DiskMetrics, diskMetric)
	}

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

// formatBytes converts bytes to a human-readable string
func formatBytes(bytes uint64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := uint64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.2f %ciB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// String returns a human-readable representation of the metrics
func (m *SystemMetrics) String() string {
	cpuStr := "CPU Usage:"
	for i, usage := range m.CPUUsage {
		cpuStr += fmt.Sprintf(" CPU%d: %.2f%%", i, usage)
	}

	memoryStr := fmt.Sprintf(
		"Memory Usage: %.2f%% (Used: %s, Free: %s, Total: %s)\n"+
			"Swap Usage: %.2f%% (Used: %s, Free: %s, Total: %s)",
		m.MemoryUsage,
		formatBytes(m.UsedMemory),
		formatBytes(m.FreeMemory),
		formatBytes(m.TotalMemory),
		m.SwapUsage,
		formatBytes(m.SwapUsed),
		formatBytes(m.SwapFree),
		formatBytes(m.SwapTotal),
	)

	diskStr := "\nDisk Usage:"
	for _, disk := range m.DiskMetrics {
		diskStr += fmt.Sprintf("\n  %s (%s): %.2f%% (Used: %s, Free: %s, Total: %s)",
			disk.Path,
			disk.FSType,
			disk.UsagePercent,
			formatBytes(disk.Used),
			formatBytes(disk.Free),
			formatBytes(disk.Total),
		)
	}

	return fmt.Sprintf(
		"System Metrics [%s]\n"+
			"System Type: %s\n"+
			"Number of CPUs: %d\n"+
			"%s\n"+
			"Load Averages: 1min: %.2f, 5min: %.2f, 15min: %.2f\n"+
			"%s%s",
		m.Timestamp.Format(time.RFC3339),
		m.SystemType,
		m.NumCPU,
		cpuStr,
		m.LoadAvg1,
		m.LoadAvg5,
		m.LoadAvg15,
		memoryStr,
		diskStr,
	)
}
