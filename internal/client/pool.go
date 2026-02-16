package client

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"math"
	"math/rand"
	"net"
	"sort"
	"strings"
	"sync"
	"time"
)

type IPStats struct {
	IP          string
	Latency     time.Duration
	TCPLatency  time.Duration
	AppLatency  time.Duration
	Quality     float64
	LastCheck   time.Time
	Fails       int
	Healthy     bool
	Successes   int
	LastRuntime time.Time
}

type AddressPool struct {
	configAddrs []string
	targetPort  int
	targetHost  string
	targetTLS   bool

	candidates map[string]*IPStats
	sortedIPs  []string

	mu       sync.RWMutex
	stopCh   chan struct{}
	stopOnce sync.Once
}

func NewAddressPool(addrs []string, port int, host string, tlsEnabled bool) (*AddressPool, error) {
	pool := &AddressPool{
		configAddrs: addrs,
		targetPort:  port,
		targetHost:  strings.TrimSpace(host),
		targetTLS:   tlsEnabled,
		candidates:  make(map[string]*IPStats),
		stopCh:      make(chan struct{}),
	}

	pool.refreshCandidates()
	go pool.checkLoop()
	return pool, nil
}

func (p *AddressPool) refreshCandidates() {
	p.mu.Lock()
	defer p.mu.Unlock()

	const maxCandidates = 1000
	if len(p.candidates) >= maxCandidates {
		return
	}

	for _, addr := range p.configAddrs {
		if _, ipnet, err := net.ParseCIDR(addr); err == nil {
			for i := 0; i < 5 && len(p.candidates) < maxCandidates; i++ {
				newIP := randomIPInSubnet(ipnet)
				ipStr := newIP.String()
				if _, exists := p.candidates[ipStr]; !exists {
					p.candidates[ipStr] = &IPStats{IP: ipStr}
				}
			}
			continue
		}

		if _, exists := p.candidates[addr]; !exists && len(p.candidates) < maxCandidates {
			p.candidates[addr] = &IPStats{IP: addr}
		}
	}
}

func (p *AddressPool) checkLoop() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-p.stopCh:
			return
		default:
		}

		p.refreshCandidates()

		p.mu.RLock()
		checkList := make([]string, 0, len(p.candidates))
		for ip := range p.candidates {
			checkList = append(checkList, ip)
		}
		p.mu.RUnlock()

		type result struct {
			IP      string
			TCP     time.Duration
			App     time.Duration
			Quality float64
			Alive   bool
		}

		results := make(chan result, len(checkList))
		var wg sync.WaitGroup
		sem := make(chan struct{}, 40)

		for _, ip := range checkList {
			wg.Add(1)
			go func(target string) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()

				tcpLatency, appLatency, ok := probeEndpointQuality(target, p.targetPort, p.targetHost, p.targetTLS)
				q := qualityScore(tcpLatency, appLatency, ok, 0)
				results <- result{
					IP:      target,
					TCP:     tcpLatency,
					App:     appLatency,
					Quality: q,
					Alive:   ok,
				}
			}(ip)
		}

		wg.Wait()
		close(results)

		p.mu.Lock()
		active := make([]string, 0, len(checkList))

		for res := range results {
			stats, exists := p.candidates[res.IP]
			if !exists {
				continue
			}

			stats.LastCheck = time.Now()
			if res.Alive {
				stats.Healthy = true
				stats.TCPLatency = res.TCP
				stats.AppLatency = res.App
				stats.Latency = res.TCP + res.App
				stats.Fails = 0
				stats.Quality = res.Quality
				active = append(active, res.IP)
				continue
			}

			stats.Healthy = false
			stats.Fails++
			stats.Quality = qualityScore(res.TCP, res.App, false, stats.Fails)
			if stats.Fails > 3 {
				delete(p.candidates, res.IP)
			}
		}

		sort.Slice(active, func(i, j int) bool {
			a := p.candidates[active[i]]
			b := p.candidates[active[j]]
			if a.Quality == b.Quality {
				return a.Latency < b.Latency
			}
			return a.Quality < b.Quality
		})
		p.sortedIPs = active

		if len(active) > 0 {
			best := p.candidates[active[0]]
			fmt.Printf("\r\033[K[%s] Active IPs: %d | Best: %s (tcp=%v app=%v)",
				time.Now().Format("15:04:05"),
				len(active),
				best.IP,
				best.TCPLatency,
				best.AppLatency,
			)
		} else {
			fmt.Printf("\r\033[K[%s] Warning: No quality-healthy IPs available.", time.Now().Format("15:04:05"))
		}
		p.mu.Unlock()

		select {
		case <-p.stopCh:
			return
		case <-ticker.C:
		}
	}
}

func qualityScore(tcpLatency, appLatency time.Duration, ok bool, fails int) float64 {
	base := float64((tcpLatency + appLatency).Microseconds())
	if base == 0 {
		base = float64((3 * time.Second).Microseconds())
	}
	if !ok {
		base += float64((2 * time.Second).Microseconds())
	}
	if fails > 0 {
		base += float64(fails) * float64((250 * time.Millisecond).Microseconds())
	}
	return base
}

func probeEndpointQuality(ip string, port int, host string, tlsEnabled bool) (tcpLatency, appLatency time.Duration, ok bool) {
	timeout := 2 * time.Second
	address := net.JoinHostPort(ip, fmt.Sprintf("%d", port))

	start := time.Now()
	conn, err := net.DialTimeout("tcp", address, timeout)
	if err != nil {
		return 0, 0, false
	}
	tcpLatency = time.Since(start)
	defer conn.Close()

	probeConn := conn
	if tlsEnabled {
		serverName := strings.TrimSpace(host)
		if serverName == "" {
			serverName = ip
		}
		tlsConn := tls.Client(conn, &tls.Config{
			ServerName:         serverName,
			InsecureSkipVerify: serverName == ip,
		})
		_ = tlsConn.SetDeadline(time.Now().Add(timeout))
		if err := tlsConn.Handshake(); err != nil {
			return tcpLatency, 0, false
		}
		probeConn = tlsConn
	}

	reqHost := strings.TrimSpace(host)
	if reqHost == "" {
		reqHost = ip
	}
	_ = probeConn.SetDeadline(time.Now().Add(timeout))
	startApp := time.Now()
	_, err = fmt.Fprintf(probeConn, "HEAD /download?session_id=quality HTTP/1.1\r\nHost: %s\r\nConnection: close\r\n\r\n", reqHost)
	if err != nil {
		return tcpLatency, 0, false
	}

	reader := bufio.NewReader(probeConn)
	line, err := reader.ReadString('\n')
	if err != nil {
		return tcpLatency, 0, false
	}
	if !strings.HasPrefix(line, "HTTP/") {
		return tcpLatency, 0, false
	}

	appLatency = time.Since(startApp)
	return tcpLatency, appLatency, true
}

func (p *AddressPool) Stop() {
	p.stopOnce.Do(func() {
		close(p.stopCh)
	})
}

func (p *AddressPool) PickBest() string {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if len(p.sortedIPs) == 0 {
		if len(p.candidates) > 0 {
			for ip := range p.candidates {
				return ip
			}
		}
		if len(p.configAddrs) > 0 {
			return p.configAddrs[0]
		}
		return "127.0.0.1"
	}

	topN := 3
	if len(p.sortedIPs) < topN {
		topN = len(p.sortedIPs)
	}
	return p.sortedIPs[rand.Intn(topN)]
}

func (p *AddressPool) ReportRuntimeResult(ip string, success bool, latency time.Duration) {
	p.mu.Lock()
	defer p.mu.Unlock()

	stats, ok := p.candidates[ip]
	if !ok {
		return
	}
	stats.LastRuntime = time.Now()
	if success {
		stats.Successes++
		if stats.Fails > 0 {
			stats.Fails--
		}
		if latency > 0 {
			if stats.AppLatency == 0 {
				stats.AppLatency = latency
			} else {
				stats.AppLatency = ewmaDuration(stats.AppLatency, latency, 0.2)
			}
			stats.Latency = stats.TCPLatency + stats.AppLatency
		}
		stats.Healthy = true
		stats.Quality = qualityScore(stats.TCPLatency, stats.AppLatency, true, stats.Fails)
		return
	}

	stats.Fails++
	stats.Healthy = false
	stats.Quality = qualityScore(stats.TCPLatency, stats.AppLatency, false, stats.Fails)
}

func ewmaDuration(prev, curr time.Duration, alpha float64) time.Duration {
	if prev <= 0 {
		return curr
	}
	p := float64(prev)
	c := float64(curr)
	v := alpha*c + (1-alpha)*p
	return time.Duration(math.Round(v))
}

func randomIPInSubnet(n *net.IPNet) net.IP {
	ip := make(net.IP, len(n.IP))
	copy(ip, n.IP)

	_, bits := n.Mask.Size()
	if bits == 32 {
		randBytes := make([]byte, 4)
		rand.Read(randBytes)
		for i := 0; i < len(ip); i++ {
			ip[i] = (ip[i] & n.Mask[i]) | (randBytes[i] & ^n.Mask[i])
		}
	}
	return ip
}
