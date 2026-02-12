package node

import (
	"bufio"
	"encoding/hex"
	"math"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"aurora-kvm-agent/internal/model"
)

type nodeNetworkAggregate struct {
	NetRxBytes uint64
	NetTxBytes uint64

	NetTotalRxBytesPerSec   float64
	NetTotalTxBytesPerSec   float64
	NetTotalRxPacketsPerSec float64
	NetTotalTxPacketsPerSec float64
	NetTotalConnections     uint64
	NetConntrackCount       uint64
	NetConntrackMax         uint64
}

type tcpStateCounters struct {
	Established uint64
	SynSent     uint64
	SynRecv     uint64
	FinWait1    uint64
	FinWait2    uint64
	TimeWait    uint64
	Close       uint64
	CloseWait   uint64
	LastAck     uint64
	Listen      uint64
	Closing     uint64
}

func (c *tcpStateCounters) total() uint64 {
	return c.Established + c.SynSent + c.SynRecv + c.FinWait1 + c.FinWait2 + c.TimeWait + c.Close + c.CloseWait + c.LastAck + c.Listen + c.Closing
}

type udpCounters struct {
	InDatagrams  uint64
	OutDatagrams uint64
	InErrors     uint64
	NoPorts      uint64
}

type ipCounters struct {
	InPackets  uint64
	OutPackets uint64
	InErrors   uint64
	OutErrors  uint64
	Forwarded  uint64
}

func (r *NodeMetricsReader) collectNodeNetworkMetrics(now time.Time) ([]model.NodeNetworkInterfaceMetrics, nodeNetworkAggregate) {
	netifs := readNetworkInterfaces()
	if len(netifs) == 0 {
		return []model.NodeNetworkInterfaceMetrics{}, nodeNetworkAggregate{}
	}

	ifaceStats := make(map[string]netIfCounterSnapshot, len(netifs))
	var currentRxBytesTotal uint64
	var currentTxBytesTotal uint64
	var currentRxPacketsTotal uint64
	var currentTxPacketsTotal uint64

	for i := range netifs {
		n := &netifs[i]
		ifaceStats[n.Name] = netIfCounterSnapshot{
			RxBytes:   n.RxBytesTotal,
			TxBytes:   n.TxBytesTotal,
			RxPackets: n.RxPacketsTotal,
			TxPackets: n.TxPacketsTotal,
		}
		currentRxBytesTotal += n.RxBytesTotal
		currentTxBytesTotal += n.TxBytesTotal
		currentRxPacketsTotal += n.RxPacketsTotal
		currentTxPacketsTotal += n.TxPacketsTotal
	}

	var (
		prevStats map[string]netIfCounterSnapshot
		elapsed   float64
	)
	r.mu.Lock()
	if r.hasPrevNetStats {
		prevStats = r.prevNetStats
		elapsed = now.Sub(r.prevNetAt).Seconds()
	}
	r.prevNetStats = ifaceStats
	r.prevNetAt = now
	r.hasPrevNetStats = true
	r.mu.Unlock()

	if elapsed > 0 && prevStats != nil {
		for i := range netifs {
			n := &netifs[i]
			prev, ok := prevStats[n.Name]
			if !ok {
				continue
			}
			n.RxBytesPerSec = float64(deltaCounter(n.RxBytesTotal, prev.RxBytes)) / elapsed
			n.TxBytesPerSec = float64(deltaCounter(n.TxBytesTotal, prev.TxBytes)) / elapsed
			n.RxPacketsPerSec = float64(deltaCounter(n.RxPacketsTotal, prev.RxPackets)) / elapsed
			n.TxPacketsPerSec = float64(deltaCounter(n.TxPacketsTotal, prev.TxPackets)) / elapsed
			n.BandwidthUtilPercent = computeBandwidthUtilPercent(n.SpeedMbps, n.RxBytesPerSec, n.TxBytesPerSec)
			n.RxLatencyMs = estimateNICLatencyMs(n.RxQueueLength, n.SpeedMbps)
			n.TxLatencyMs = estimateNICLatencyMs(n.TxQueueLength, n.SpeedMbps)
		}
	}

	tcpGlobal, tcpByInterface := readTCPStateCounters(netifs)
	udpGlobal, ipGlobal := readNodeProtocolCounters()
	applyProtocolCountersToInterfaces(netifs, tcpByInterface, udpGlobal, ipGlobal)

	conntrackCount, conntrackMax := readConntrackStats()

	global := nodeNetworkAggregate{
		NetRxBytes:            currentRxBytesTotal,
		NetTxBytes:            currentTxBytesTotal,
		NetTotalConnections:   tcpGlobal.total(),
		NetConntrackCount:     conntrackCount,
		NetConntrackMax:       conntrackMax,
		NetTotalRxBytesPerSec: 0,
		NetTotalTxBytesPerSec: 0,
	}

	if elapsed > 0 && prevStats != nil {
		var prevRxBytesTotal uint64
		var prevTxBytesTotal uint64
		var prevRxPacketsTotal uint64
		var prevTxPacketsTotal uint64
		for _, prev := range prevStats {
			prevRxBytesTotal += prev.RxBytes
			prevTxBytesTotal += prev.TxBytes
			prevRxPacketsTotal += prev.RxPackets
			prevTxPacketsTotal += prev.TxPackets
		}
		global.NetTotalRxBytesPerSec = float64(deltaCounter(currentRxBytesTotal, prevRxBytesTotal)) / elapsed
		global.NetTotalTxBytesPerSec = float64(deltaCounter(currentTxBytesTotal, prevTxBytesTotal)) / elapsed
		global.NetTotalRxPacketsPerSec = float64(deltaCounter(currentRxPacketsTotal, prevRxPacketsTotal)) / elapsed
		global.NetTotalTxPacketsPerSec = float64(deltaCounter(currentTxPacketsTotal, prevTxPacketsTotal)) / elapsed
	}

	sort.Slice(netifs, func(i, j int) bool { return netifs[i].Name < netifs[j].Name })
	return netifs, global
}

func readNetworkInterfaces() []model.NodeNetworkInterfaceMetrics {
	entries, err := os.ReadDir("/sys/class/net")
	if err != nil {
		return []model.NodeNetworkInterfaceMetrics{}
	}

	netifs := make([]model.NodeNetworkInterfaceMetrics, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if shouldSkipNetworkInterface(name) {
			continue
		}
		netif := model.NodeNetworkInterfaceMetrics{
			Name: name,
		}

		base := filepath.Join("/sys/class/net", name)
		netif.MacAddress = readTextFile(filepath.Join(base, "address"))
		netif.MTU = readUintFile(filepath.Join(base, "mtu"))
		netif.SpeedMbps = readUintSignedFile(filepath.Join(base, "speed"))
		netif.Duplex = readTextFile(filepath.Join(base, "duplex"))
		netif.Operstate = readTextFile(filepath.Join(base, "operstate"))
		netif.Type = mapLinkType(readUintFile(filepath.Join(base, "type")))
		netif.Driver = readNICDriver(name)
		netif.PCIAddress = readNICPCIAddress(name)
		netif.LinkUp = readTextFile(filepath.Join(base, "carrier")) == "1" || strings.EqualFold(netif.Operstate, "up")

		netif.Bridge = readNICBridge(name)
		netif.VLANID = readNICVLANID(name)
		netif.BondMaster = readNICBondMaster(name)
		netif.BondMode, netif.BondSlaves = readNICBondInfo(name)
		if netif.BondMode == "" && netif.BondMaster != "" {
			mode, _ := readNICBondInfo(netif.BondMaster)
			netif.BondMode = mode
		}

		netif.TxQueueLength = readUintFile(filepath.Join(base, "tx_queue_len"))
		netif.RxQueueLength = readNICRXQueueLength(name)

		statsBase := filepath.Join(base, "statistics")
		netif.RxBytesTotal = readUintFile(filepath.Join(statsBase, "rx_bytes"))
		netif.TxBytesTotal = readUintFile(filepath.Join(statsBase, "tx_bytes"))
		netif.RxPacketsTotal = readUintFile(filepath.Join(statsBase, "rx_packets"))
		netif.TxPacketsTotal = readUintFile(filepath.Join(statsBase, "tx_packets"))
		netif.RxErrors = readUintFile(filepath.Join(statsBase, "rx_errors"))
		netif.TxErrors = readUintFile(filepath.Join(statsBase, "tx_errors"))
		netif.RxDropped = readUintFile(filepath.Join(statsBase, "rx_dropped"))
		netif.TxDropped = readUintFile(filepath.Join(statsBase, "tx_dropped"))
		netif.RxOverruns = readUintFile(filepath.Join(statsBase, "rx_over_errors"))
		netif.TxOverruns = readUintFile(filepath.Join(statsBase, "tx_aborted_errors"))
		netif.RxFIFOErrors = readUintFile(filepath.Join(statsBase, "rx_fifo_errors"))
		netif.TxFIFOErrors = readUintFile(filepath.Join(statsBase, "tx_fifo_errors"))
		netif.Collisions = readUintFile(filepath.Join(statsBase, "collisions"))
		netif.CarrierErrors = readUintFile(filepath.Join(statsBase, "tx_carrier_errors"))
		netif.RxCompressed = readUintFile(filepath.Join(statsBase, "rx_compressed"))
		netif.TxCompressed = readUintFile(filepath.Join(statsBase, "tx_compressed"))
		netif.Multicast = readUintFile(filepath.Join(statsBase, "multicast"))
		netif.Broadcast = readUintFile(filepath.Join(statsBase, "broadcast"))

		netif.DropsTotal = netif.RxDropped + netif.TxDropped
		netif.ErrorsTotal = netif.RxErrors + netif.TxErrors + netif.RxOverruns + netif.TxOverruns + netif.RxFIFOErrors + netif.TxFIFOErrors + netif.Collisions + netif.CarrierErrors

		netifs = append(netifs, netif)
	}

	return netifs
}

func shouldSkipNetworkInterface(name string) bool {
	name = strings.TrimSpace(name)
	if name == "" {
		return true
	}
	if name == "lo" {
		return true
	}
	if strings.HasPrefix(name, "docker") || strings.HasPrefix(name, "veth") {
		return true
	}
	return false
}

func readNICDriver(name string) string {
	link, err := os.Readlink(filepath.Join("/sys/class/net", name, "device", "driver"))
	if err != nil {
		return ""
	}
	return filepath.Base(link)
}

func readNICPCIAddress(name string) string {
	link, err := os.Readlink(filepath.Join("/sys/class/net", name, "device"))
	if err != nil {
		return ""
	}
	return filepath.Base(link)
}

func readNICBridge(name string) string {
	link, err := os.Readlink(filepath.Join("/sys/class/net", name, "brport", "bridge"))
	if err == nil {
		return filepath.Base(link)
	}
	if _, statErr := os.Stat(filepath.Join("/sys/class/net", name, "bridge")); statErr == nil {
		return name
	}
	return ""
}

func readNICVLANID(name string) uint64 {
	path := filepath.Join("/proc/net/vlan", name)
	raw, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	lines := strings.Split(string(raw), "\n")
	for _, line := range lines {
		if !strings.Contains(line, "VID:") {
			continue
		}
		parts := strings.Split(line, "VID:")
		if len(parts) < 2 {
			continue
		}
		fields := strings.Fields(strings.TrimSpace(parts[1]))
		if len(fields) == 0 {
			continue
		}
		return parseUintFlexible(fields[0])
	}
	return 0
}

func readNICBondMaster(name string) string {
	link, err := os.Readlink(filepath.Join("/sys/class/net", name, "master"))
	if err != nil {
		return ""
	}
	return filepath.Base(link)
}

func readNICBondInfo(name string) (string, []string) {
	path := filepath.Join("/proc/net/bonding", name)
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", nil
	}
	mode := ""
	slaves := make([]string, 0, 4)
	scanner := bufio.NewScanner(strings.NewReader(string(raw)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "Bonding Mode:") {
			mode = strings.TrimSpace(strings.TrimPrefix(line, "Bonding Mode:"))
			continue
		}
		if strings.HasPrefix(line, "Slave Interface:") {
			slave := strings.TrimSpace(strings.TrimPrefix(line, "Slave Interface:"))
			if slave != "" {
				slaves = append(slaves, slave)
			}
		}
	}
	sort.Strings(slaves)
	return mode, slaves
}

func readNICRXQueueLength(name string) uint64 {
	pattern := filepath.Join("/sys/class/net", name, "queues", "rx-*", "rps_flow_cnt")
	paths, err := filepath.Glob(pattern)
	if err != nil || len(paths) == 0 {
		return 0
	}
	var total uint64
	for _, path := range paths {
		total += readUintFile(path)
	}
	return total
}

func mapLinkType(raw uint64) string {
	switch raw {
	case 1:
		return "ethernet"
	case 32:
		return "infiniband"
	case 512:
		return "ppp"
	case 772:
		return "loopback"
	case 65534:
		return "none"
	default:
		return "unknown"
	}
}

func computeBandwidthUtilPercent(speedMbps uint64, rxBytesPerSec, txBytesPerSec float64) float64 {
	if speedMbps == 0 {
		return 0
	}
	capacityBytesPerSec := (float64(speedMbps) * 1_000_000) / 8
	if capacityBytesPerSec <= 0 {
		return 0
	}
	return clampPercent(((rxBytesPerSec + txBytesPerSec) / capacityBytesPerSec) * 100)
}

func estimateNICLatencyMs(queueLength, speedMbps uint64) float64 {
	if queueLength == 0 || speedMbps == 0 {
		return 0
	}
	bitsPerSec := float64(speedMbps) * 1_000_000
	if bitsPerSec <= 0 {
		return 0
	}
	const avgPacketBits = 1500 * 8
	return (float64(queueLength*avgPacketBits) / bitsPerSec) * 1000
}

func readTCPStateCounters(netifs []model.NodeNetworkInterfaceMetrics) (tcpStateCounters, map[string]tcpStateCounters) {
	ipToIfaces := mapIPToInterface(netifs)
	byInterface := make(map[string]tcpStateCounters, len(netifs))
	for _, netif := range netifs {
		byInterface[netif.Name] = tcpStateCounters{}
	}

	global := tcpStateCounters{}
	raw, err := os.ReadFile("/proc/net/tcp")
	if err != nil {
		return global, byInterface
	}
	lines := strings.Split(string(raw), "\n")
	for i, line := range lines {
		if i == 0 {
			continue
		}
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) < 4 {
			continue
		}
		localAddress := fields[1]
		stateHex := fields[3]
		localParts := strings.Split(localAddress, ":")
		if len(localParts) != 2 {
			continue
		}
		localIP := parseProcTCPIPv4(localParts[0])

		incrementTCPState(&global, stateHex)
		if localIP == "" || localIP == "0.0.0.0" {
			continue
		}
		ifaces := ipToIfaces[localIP]
		for _, iface := range ifaces {
			c := byInterface[iface]
			incrementTCPState(&c, stateHex)
			byInterface[iface] = c
		}
	}

	return global, byInterface
}

func incrementTCPState(dst *tcpStateCounters, stateHex string) {
	switch strings.ToUpper(strings.TrimSpace(stateHex)) {
	case "01":
		dst.Established++
	case "02":
		dst.SynSent++
	case "03":
		dst.SynRecv++
	case "04":
		dst.FinWait1++
	case "05":
		dst.FinWait2++
	case "06":
		dst.TimeWait++
	case "07":
		dst.Close++
	case "08":
		dst.CloseWait++
	case "09":
		dst.LastAck++
	case "0A":
		dst.Listen++
	case "0B":
		dst.Closing++
	}
}

func parseProcTCPIPv4(hexText string) string {
	hexText = strings.TrimSpace(hexText)
	if len(hexText) != 8 {
		return ""
	}
	b, err := hex.DecodeString(hexText)
	if err != nil || len(b) != 4 {
		return ""
	}
	ip := net.IPv4(b[3], b[2], b[1], b[0])
	return ip.String()
}

func mapIPToInterface(netifs []model.NodeNetworkInterfaceMetrics) map[string][]string {
	out := make(map[string][]string)
	for _, netif := range netifs {
		i, err := net.InterfaceByName(netif.Name)
		if err != nil {
			continue
		}
		addrs, err := i.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			ip := extractIPFromAddr(addr)
			if ip == "" {
				continue
			}
			out[ip] = append(out[ip], netif.Name)
		}
	}
	for ip := range out {
		sort.Strings(out[ip])
	}
	return out
}

func extractIPFromAddr(addr net.Addr) string {
	switch v := addr.(type) {
	case *net.IPNet:
		if v.IP == nil {
			return ""
		}
		if ipv4 := v.IP.To4(); ipv4 != nil {
			return ipv4.String()
		}
		return ""
	case *net.IPAddr:
		if v.IP == nil {
			return ""
		}
		if ipv4 := v.IP.To4(); ipv4 != nil {
			return ipv4.String()
		}
		return ""
	default:
		return ""
	}
}

func readNodeProtocolCounters() (udpCounters, ipCounters) {
	udp := udpCounters{}
	ip := ipCounters{}
	raw, err := os.ReadFile("/proc/net/snmp")
	if err != nil {
		return udp, ip
	}
	lines := strings.Split(string(raw), "\n")
	for i := 0; i+1 < len(lines); i++ {
		header := strings.Fields(strings.TrimSpace(lines[i]))
		value := strings.Fields(strings.TrimSpace(lines[i+1]))
		if len(header) < 2 || len(value) < 2 {
			continue
		}
		if header[0] != value[0] {
			continue
		}
		proto := strings.TrimSuffix(header[0], ":")
		keys := header[1:]
		vals := value[1:]
		if len(vals) != len(keys) {
			continue
		}
		fields := make(map[string]uint64, len(keys))
		for idx, key := range keys {
			fields[key] = parseUintFlexible(vals[idx])
		}
		switch proto {
		case "Udp":
			udp.InDatagrams = fields["InDatagrams"]
			udp.OutDatagrams = fields["OutDatagrams"]
			udp.InErrors = fields["InErrors"]
			udp.NoPorts = fields["NoPorts"]
		case "Ip":
			ip.InPackets = fields["InReceives"]
			ip.OutPackets = fields["OutRequests"]
			ip.InErrors = fields["InHdrErrors"] + fields["InAddrErrors"] + fields["InDiscards"]
			ip.OutErrors = fields["OutDiscards"] + fields["OutNoRoutes"]
			ip.Forwarded = fields["ForwDatagrams"]
		}
		i++
	}
	return udp, ip
}

func applyProtocolCountersToInterfaces(
	netifs []model.NodeNetworkInterfaceMetrics,
	tcpByInterface map[string]tcpStateCounters,
	udpGlobal udpCounters,
	ipGlobal ipCounters,
) {
	var totalPackets uint64
	for _, netif := range netifs {
		totalPackets += netif.RxPacketsTotal + netif.TxPacketsTotal
	}

	for i := range netifs {
		n := &netifs[i]
		tcp := tcpByInterface[n.Name]
		n.TCPConnectionsTotal = tcp.total()
		n.TCPEstablished = tcp.Established
		n.TCPSynSent = tcp.SynSent
		n.TCPSynRecv = tcp.SynRecv
		n.TCPFinWait1 = tcp.FinWait1
		n.TCPFinWait2 = tcp.FinWait2
		n.TCPTimeWait = tcp.TimeWait
		n.TCPClose = tcp.Close
		n.TCPCloseWait = tcp.CloseWait
		n.TCPLastAck = tcp.LastAck
		n.TCPListen = tcp.Listen
		n.TCPClosing = tcp.Closing

		weight := 0.0
		if totalPackets > 0 {
			weight = float64(n.RxPacketsTotal+n.TxPacketsTotal) / float64(totalPackets)
		}
		n.UDPInDatagrams = weightedCounter(udpGlobal.InDatagrams, weight)
		n.UDPOutDatagrams = weightedCounter(udpGlobal.OutDatagrams, weight)
		n.UDPInErrors = weightedCounter(udpGlobal.InErrors, weight)
		n.UDPNoPorts = weightedCounter(udpGlobal.NoPorts, weight)

		n.IPInPackets = weightedCounter(ipGlobal.InPackets, weight)
		n.IPOutPackets = weightedCounter(ipGlobal.OutPackets, weight)
		n.IPInErrors = weightedCounter(ipGlobal.InErrors, weight)
		n.IPOutErrors = weightedCounter(ipGlobal.OutErrors, weight)
		n.IPForwarded = weightedCounter(ipGlobal.Forwarded, weight)
	}
}

func weightedCounter(total uint64, weight float64) uint64 {
	if total == 0 || weight <= 0 {
		return 0
	}
	return uint64(math.Round(float64(total) * weight))
}

func readConntrackStats() (uint64, uint64) {
	count := readUintFile("/proc/sys/net/netfilter/nf_conntrack_count")
	max := readUintFile("/proc/sys/net/netfilter/nf_conntrack_max")
	return count, max
}

func readUintFile(path string) uint64 {
	raw, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	return parseUintFlexible(string(raw))
}

func readUintSignedFile(path string) uint64 {
	raw, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	text := strings.TrimSpace(string(raw))
	if text == "" {
		return 0
	}
	if strings.HasPrefix(text, "-") {
		return 0
	}
	return parseUintFlexible(text)
}
