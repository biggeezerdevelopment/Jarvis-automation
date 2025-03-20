package monitoring

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/load"
	"github.com/shirou/gopsutil/v3/mem"
	gopsutilnet "github.com/shirou/gopsutil/v3/net"
)

// SystemMetrics represents system metrics data
type SystemMetrics struct {
	Timestamp   time.Time       `json:"timestamp"`
	CpuMetrics  CpuMetric       `json:"cpu_metrics"`  // Metrics for the CPU
	MemMetrics  MemMetric       `json:"mem_metrics"`  // Metrics for the memory
	OsMetrics   OsMetric        `json:"os_metrics"`   // Metrics for the operating system
	DiskMetrics []DiskMetric    `json:"disk_metrics"` // Metrics for each disk partition
	NetMetrics  []NetworkMetric `json:"net_metrics"`  // Metrics for each network interface
}

type CpuMetric struct {
	CPUUsage  []float64 `json:"cpu_usage"`   // Per CPU usage percentage
	LoadAvg1  float64   `json:"load_avg_1"`  // 1 minute load average
	LoadAvg5  float64   `json:"load_avg_5"`  // 5 minute load average
	LoadAvg15 float64   `json:"load_avg_15"` // 15 minute load average
	NumCPU    int       `json:"num_cpu"`     // Number of CPUs
}

// MemMetric represents metrics for the memory
type MemMetric struct {
	TotalMemory uint64  `json:"total_memory"` // Total physical memory in bytes
	UsedMemory  uint64  `json:"used_memory"`  // Used physical memory in bytes
	FreeMemory  uint64  `json:"free_memory"`  // Free physical memory in bytes
	MemoryUsage float64 `json:"memory_usage"` // Memory usage percentage
	SwapTotal   uint64  `json:"swap_total"`   // Total swap space in bytes
	SwapUsed    uint64  `json:"swap_used"`    // Used swap space in bytes
	SwapFree    uint64  `json:"swap_free"`    // Free swap space in bytes
	SwapUsage   float64 `json:"swap_usage"`   // Swap usage percentage
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

// NetworkMetric represents metrics for a single network interface
type NetworkMetric struct {
	Name        string   `json:"name"`         // Interface name
	BytesSent   uint64   `json:"bytes_sent"`   // Total bytes sent
	BytesRecv   uint64   `json:"bytes_recv"`   // Total bytes received
	PacketsSent uint64   `json:"packets_sent"` // Total packets sent
	PacketsRecv uint64   `json:"packets_recv"` // Total packets received
	ErrorsIn    uint64   `json:"errors_in"`    // Total errors received
	ErrorsOut   uint64   `json:"errors_out"`   // Total errors sent
	DropIn      uint64   `json:"drop_in"`      // Total dropped packets received
	DropOut     uint64   `json:"drop_out"`     // Total dropped packets sent
	Speed       float64  `json:"speed"`        // Interface speed in Mbps
	MTU         int      `json:"mtu"`          // Maximum Transmission Unit
	IPAddresses []string `json:"ip_addresses"` // List of IP addresses
	MACAddress  string   `json:"mac_address"`  // MAC address
	Gateway     string   `json:"gateway"`      // Default gateway
}

type OsMetric struct {
	OsType     string `json:"os_type"`
	OSName     string `json:"os_name"`
	OSVersion  string `json:"os_version"`
	OSArch     string `json:"os_arch"`
	OSPlatform string `json:"os_platform"`
	OSHostname string `json:"os_hostname"`
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

func getOSVersion() (string, error) {
	osName := runtime.GOOS

	switch osName {
	case "windows":
		cmd := exec.Command("cmd", "/c", "ver")
		output, err := cmd.CombinedOutput()
		if err != nil {
			return string(output), err
		}
		return strings.ReplaceAll(string(output), "\r\n", ""), nil
	case "linux":
		cmd := exec.Command("uname", "-r")
		output, err := cmd.CombinedOutput()
		if err != nil {
			return string(output), err
		}
		return strings.ReplaceAll(string(output), "\r\n", ""), nil
	case "darwin":
		cmd := exec.Command("sw_vers")
		output, err := cmd.CombinedOutput()
		if err != nil {
			return string(output), err
		}
		return strings.ReplaceAll(string(output), "\r\n", ""), nil

	default:
		return "", fmt.Errorf("unsupported operating system: %s", osName)
	}
}

// GetMetrics gathers current system metrics
func (m *Monitor) GetMetrics() (*SystemMetrics, error) {
	metrics := &SystemMetrics{
		Timestamp: time.Now(),
	}

	// Get OS information
	hostname, err := os.Hostname()
	if err != nil {
		return nil, fmt.Errorf("failed to get hostname: %v", err)
	}
	osversion, err := getOSVersion()
	if err != nil {
		return nil, fmt.Errorf("failed to get OS version: %v", err)
	}
	metrics.OsMetrics = OsMetric{
		OsType:     runtime.GOOS,
		OSName:     runtime.GOOS,
		OSVersion:  osversion,
		OSArch:     runtime.GOARCH,
		OSPlatform: runtime.GOOS + "/" + runtime.GOARCH,
		OSHostname: hostname,
	}

	cpuMetrics := &CpuMetric{
		NumCPU: runtime.NumCPU(),
	}

	// Get CPU usage percentages
	cpuPercentages, err := cpu.Percent(m.interval, true)
	if err != nil {
		return nil, fmt.Errorf("failed to get CPU usage: %v", err)
	}
	cpuMetrics.CPUUsage = cpuPercentages

	// Get memory statistics
	memStats, err := mem.VirtualMemory()
	if err != nil {
		return nil, fmt.Errorf("failed to get memory stats: %v", err)
	}

	// Get swap statistics
	swapStats, err := mem.SwapMemory()
	if err != nil {
		return nil, fmt.Errorf("failed to get swap stats: %v", err)
	}

	memMetric := MemMetric{
		TotalMemory: memStats.Total,
		UsedMemory:  memStats.Used,
		FreeMemory:  memStats.Free,
		MemoryUsage: memStats.UsedPercent,
		SwapTotal:   swapStats.Total,
		SwapUsed:    swapStats.Used,
		SwapFree:    swapStats.Free,
		SwapUsage:   swapStats.UsedPercent,
	}

	metrics.MemMetrics = memMetric
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
		cpuMetrics.LoadAvg1 = loadAvg.Load1
		cpuMetrics.LoadAvg5 = loadAvg.Load5
		cpuMetrics.LoadAvg15 = loadAvg.Load15
	} else {
		// For Windows, we'll use CPU usage as a proxy for load average
		avgCPU := 0.0
		for _, usage := range cpuPercentages {
			avgCPU += usage
		}
		avgCPU /= float64(len(cpuPercentages))
		cpuMetrics.LoadAvg1 = avgCPU
		cpuMetrics.LoadAvg5 = avgCPU
		cpuMetrics.LoadAvg15 = avgCPU
	}

	// Get network interface statistics
	netStats, err := gopsutilnet.IOCounters(true) // true means per interface
	if err != nil {
		return nil, fmt.Errorf("failed to get network stats: %v", err)
	}

	metrics.NetMetrics = make([]NetworkMetric, 0, len(netStats))
	for _, stat := range netStats {
		netMetric := NetworkMetric{
			Name:        stat.Name,
			BytesSent:   stat.BytesSent,
			BytesRecv:   stat.BytesRecv,
			PacketsSent: stat.PacketsSent,
			PacketsRecv: stat.PacketsRecv,
			ErrorsIn:    stat.Errin,
			ErrorsOut:   stat.Errout,
			DropIn:      stat.Dropin,
			DropOut:     stat.Dropout,
		}

		// Get interface details
		iface, err := net.InterfaceByName(stat.Name)
		if err == nil {
			netMetric.MTU = iface.MTU
			netMetric.MACAddress = iface.HardwareAddr.String()

			// Get IP addresses
			addrs, err := iface.Addrs()
			if err == nil {
				netMetric.IPAddresses = make([]string, 0, len(addrs))
				for _, addr := range addrs {
					netMetric.IPAddresses = append(netMetric.IPAddresses, addr.String())
				}
			}

			// Get default gateway (platform specific)
			if runtime.GOOS == "windows" {
				// On Windows, we can get the gateway from the routing table
				// This is a simplified version - you might want to add more robust gateway detection
				if len(netMetric.IPAddresses) > 0 {
					netMetric.Gateway = "0.0.0.0" // Default gateway placeholder
				}
			} else {
				// On Unix-like systems, we can try to get the gateway from the routing table
				netMetric.Gateway = "0.0.0.0" // Default gateway placeholder
			}
		}

		metrics.NetMetrics = append(metrics.NetMetrics, netMetric)
	}

	metrics.CpuMetrics = *cpuMetrics

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
	for i, usage := range m.CpuMetrics.CPUUsage {
		cpuStr += fmt.Sprintf(" CPU%d: %.2f%%", i, usage)
	}

	memoryStr := fmt.Sprintf(
		"Memory Usage: %.2f%% (Used: %s, Free: %s, Total: %s)\n"+
			"Swap Usage: %.2f%% (Used: %s, Free: %s, Total: %s)",
		m.MemMetrics.MemoryUsage,
		formatBytes(m.MemMetrics.UsedMemory),
		formatBytes(m.MemMetrics.FreeMemory),
		formatBytes(m.MemMetrics.TotalMemory),
		m.MemMetrics.SwapUsage,
		formatBytes(m.MemMetrics.SwapUsed),
		formatBytes(m.MemMetrics.SwapFree),
		formatBytes(m.MemMetrics.SwapTotal),
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

	netStr := "\nNetwork Interfaces:"
	for _, net := range m.NetMetrics {
		netStr += fmt.Sprintf("\n  %s:", net.Name)
		netStr += fmt.Sprintf("\n    Sent: %s (%d packets)", formatBytes(net.BytesSent), net.PacketsSent)
		netStr += fmt.Sprintf("\n    Received: %s (%d packets)", formatBytes(net.BytesRecv), net.PacketsRecv)
		if net.ErrorsIn > 0 || net.ErrorsOut > 0 {
			netStr += fmt.Sprintf("\n    Errors: In=%d, Out=%d", net.ErrorsIn, net.ErrorsOut)
		}
		if net.DropIn > 0 || net.DropOut > 0 {
			netStr += fmt.Sprintf("\n    Dropped: In=%d, Out=%d", net.DropIn, net.DropOut)
		}
		if net.MTU > 0 {
			netStr += fmt.Sprintf("\n    MTU: %d", net.MTU)
		}
		if net.MACAddress != "" {
			netStr += fmt.Sprintf("\n    MAC: %s", net.MACAddress)
		}
		if len(net.IPAddresses) > 0 {
			netStr += fmt.Sprintf("\n    IPs: %v", net.IPAddresses)
		}
		if net.Gateway != "" {
			netStr += fmt.Sprintf("\n    Gateway: %s", net.Gateway)
		}
	}

	return fmt.Sprintf(
		"System Metrics [%s]\n"+
			"Number of CPUs: %d\n"+
			"%s\n"+
			"Load Averages: 1min: %.2f, 5min: %.2f, 15min: %.2f\n"+
			"%s%s%s",
		m.Timestamp.Format(time.RFC3339),
		m.CpuMetrics.NumCPU,
		cpuStr,
		m.CpuMetrics.LoadAvg1,
		m.CpuMetrics.LoadAvg5,
		m.CpuMetrics.LoadAvg15,
		memoryStr,
		diskStr,
		netStr,
	)
}
