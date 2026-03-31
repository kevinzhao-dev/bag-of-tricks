package collector

import (
	"bufio"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// Stats holds a single snapshot of network interface counters.
type Stats struct {
	RxBytes   uint64
	TxBytes   uint64
	Timestamp time.Time
}

// Delta holds the computed difference between two snapshots.
type Delta struct {
	RxBytes  uint64
	TxBytes  uint64
	Duration time.Duration
}

// RxRate returns download speed in bytes per second.
func (d Delta) RxRate() float64 {
	if d.Duration.Seconds() == 0 {
		return 0
	}
	return float64(d.RxBytes) / d.Duration.Seconds()
}

// TxRate returns upload speed in bytes per second.
func (d Delta) TxRate() float64 {
	if d.Duration.Seconds() == 0 {
		return 0
	}
	return float64(d.TxBytes) / d.Duration.Seconds()
}

// Collector periodically reads network interface statistics.
type Collector struct {
	iface string
	prev  *Stats
}

// New creates a Collector for the given network interface.
func New(iface string) *Collector {
	return &Collector{iface: iface}
}

// Collect reads the current counters and returns the delta since last call.
// On the first call, delta will be nil.
func (c *Collector) Collect() (*Delta, *Stats, error) {
	stats, err := c.getStats()
	if err != nil {
		return nil, nil, err
	}

	if c.prev == nil {
		c.prev = stats
		return nil, stats, nil
	}

	duration := stats.Timestamp.Sub(c.prev.Timestamp)

	// Skip sample if time gap > 5s (sleep/wake detection)
	if duration > 5*time.Second {
		c.prev = stats
		return nil, stats, nil
	}

	delta := &Delta{
		Duration: duration,
	}

	// Handle counter reset (reboot): if current < prev, use current as-is
	if stats.RxBytes >= c.prev.RxBytes {
		delta.RxBytes = stats.RxBytes - c.prev.RxBytes
	} else {
		delta.RxBytes = stats.RxBytes
	}
	if stats.TxBytes >= c.prev.TxBytes {
		delta.TxBytes = stats.TxBytes - c.prev.TxBytes
	} else {
		delta.TxBytes = stats.TxBytes
	}

	c.prev = stats
	return delta, stats, nil
}

// getStats shells out to `netstat -ib` and parses the result.
func (c *Collector) getStats() (*Stats, error) {
	output, err := exec.Command("netstat", "-ib").Output()
	if err != nil {
		return nil, fmt.Errorf("netstat: %w", err)
	}
	return parseNetstat(output, c.iface)
}

// parseNetstat parses netstat -ib output and extracts Ibytes/Obytes for the given interface.
// It looks for the row with <Link#...> in the Network column.
func parseNetstat(data []byte, iface string) (*Stats, error) {
	scanner := bufio.NewScanner(strings.NewReader(string(data)))

	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) < 11 {
			continue
		}

		name := fields[0]
		network := fields[2]

		if name == iface && strings.HasPrefix(network, "<Link#") {
			ibytes, err := strconv.ParseUint(fields[6], 10, 64)
			if err != nil {
				return nil, fmt.Errorf("parse Ibytes: %w", err)
			}
			obytes, err := strconv.ParseUint(fields[9], 10, 64)
			if err != nil {
				return nil, fmt.Errorf("parse Obytes: %w", err)
			}
			return &Stats{
				RxBytes:   ibytes,
				TxBytes:   obytes,
				Timestamp: time.Now(),
			}, nil
		}
	}

	return nil, fmt.Errorf("interface %q not found in netstat output", iface)
}
