package client

import (
	"fmt"
	"math/rand"
	"net"
	"sort"
	"sync"
	"time"
)

type IPStats struct {
	IP        string
	Latency   time.Duration
	LastCheck time.Time
	Fails     int
	Healthy   bool
}

type AddressPool struct {
	configAddrs []string // Original config entries (IPs or CIDRs)
	targetPort  int      // Server port to check

	candidates map[string]*IPStats // Active working set of IPs
	sortedIPs  []string            // IPs sorted by latency (fastest first)

	mu sync.RWMutex
}

func NewAddressPool(addrs []string, port int) (*AddressPool, error) {
	pool := &AddressPool{
		configAddrs: addrs,
		targetPort:  port,
		candidates:  make(map[string]*IPStats),
	}

	// Initial population
	pool.refreshCandidates()

	// Start background checker
	go pool.checkLoop()

	return pool, nil
}

// refreshCandidates ensures we have enough candidates.
// It parses CIDRs and picks random IPs if needed.
func (p *AddressPool) refreshCandidates() {
	p.mu.Lock()
	defer p.mu.Unlock()

	// If we already have enough healthy candidates, maybe we don't need to add more?
	// But we should always ensure we have a mix if the list is small.
	// For now, let's just re-scan configAddrs and ensure we have some base set.

	// Limit total candidates to avoid memory explosion
	const maxCandidates = 1000

	if len(p.candidates) >= maxCandidates {
		return
	}

	for _, addr := range p.configAddrs {
		if ip, ipnet, err := net.ParseCIDR(addr); err == nil {
			// It's a CIDR
			// Add a few random IPs from this CIDR
			// If it's a small CIDR/Subnet, maybe add all?
			// For MVP, just add 5 random ones each refresh cycle up to limit
			for i := 0; i < 5; i++ {
				newIP := randomIPInSubnet(ipnet)
				ipStr := newIP.String()
				if _, exists := p.candidates[ipStr]; !exists {
					p.candidates[ipStr] = &IPStats{
						IP: ipStr,
					}
				}
			}
			// Always ensure the base IP (if meaningful) or just randoms?
			// net.ParseCIDR returns the IP part too.
			if ip != nil {
				// Often the IP part of a CIDR string is the network address, which might not be a valid host.
				// But sometimes it is "1.2.3.4/24".
				// Let's ignore the base IP unless it's a /32, which falls into the else block usually?
				// Actually ParseCIDR("1.2.3.4/32") works.
			}

		} else {
			// Assume it's a single IP or Hostname
			if _, exists := p.candidates[addr]; !exists {
				p.candidates[addr] = &IPStats{
					IP: addr,
				}
			}
		}
	}
}

func (p *AddressPool) checkLoop() {
	ticker := time.NewTicker(10 * time.Second) // Check every 10s
	for {
		// 1. Refresh candidates (add new ones if needed)
		p.refreshCandidates()

		// 2. Snapshot candidates for checking to avoid holding lock during network ops
		p.mu.RLock()
		checkList := make([]string, 0, len(p.candidates))
		for ip := range p.candidates {
			checkList = append(checkList, ip)
		}
		p.mu.RUnlock()

		// 3. Check each candidate
		type result struct {
			IP      string
			Latency time.Duration
			Alive   bool
		}
		results := make(chan result, len(checkList))
		var wg sync.WaitGroup

		// Limit concurrency
		sem := make(chan struct{}, 50) // Max 50 concurrent checks

		for _, ip := range checkList {
			wg.Add(1)
			go func(target string) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()

				start := time.Now()
				alive := isHealthy(target, p.targetPort)
				latency := time.Since(start)

				results <- result{IP: target, Latency: latency, Alive: alive}
			}(ip)
		}

		wg.Wait()
		close(results)

		// 4. Update stats and sort
		p.mu.Lock()
		var active []string

		for res := range results {
			stats, exists := p.candidates[res.IP]
			if !exists {
				continue
			}

			if res.Alive {
				stats.Healthy = true
				stats.Latency = res.Latency
				stats.LastCheck = time.Now()
				stats.Fails = 0
				active = append(active, res.IP)
			} else {
				stats.Healthy = false
				stats.Fails++
				// Remove if too many fails?
				if stats.Fails > 3 {
					delete(p.candidates, res.IP)
				}
			}
		}

		// Sort active IPs by latency
		sort.Slice(active, func(i, j int) bool {
			return p.candidates[active[i]].Latency < p.candidates[active[j]].Latency
		})
		p.sortedIPs = active
		p.mu.Unlock()


		// Log status occasionally
		if len(active) > 0 {
			bestIP := active[0]
			bestLatency := p.candidates[bestIP].Latency
			fmt.Printf("\r\033[K[%s] Active IPs: %d | Best: %s (%v)", 
				time.Now().Format("15:04:05"), 
				len(active), 
				bestIP, 
				bestLatency)
		} else {
			fmt.Printf("\r\033[K[%s] Warning: No healthy IPs available.", time.Now().Format("15:04:05"))
		}

		<-ticker.C
	}
}

// PickBest returns one of the best available IPs.
func (p *AddressPool) PickBest() string {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if len(p.sortedIPs) == 0 {
		// Fallback: Pick a random one from config or candidates?
		// If we have candidates (even if unchecked/unhealthy), try one?
		if len(p.candidates) > 0 {
			// iterate map to get a random one
			for ip := range p.candidates {
				return ip
			}
		}
		// Last resort
		if len(p.configAddrs) > 0 {
			// Just return the first config addr (stripping CIDR if needed, but for now just raw)
			return p.configAddrs[0]
		}
		return "127.0.0.1"
	}

	// Load balancing: Pick randomly from top 3 (or fewer if not enough)
	topN := 3
	if len(p.sortedIPs) < topN {
		topN = len(p.sortedIPs)
	}

	idx := rand.Intn(topN)
	return p.sortedIPs[idx]
}

func isHealthy(ip string, port int) bool {
	timeout := 2 * time.Second
	address := net.JoinHostPort(ip, fmt.Sprintf("%d", port))
	conn, err := net.DialTimeout("tcp", address, timeout)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

func randomIPInSubnet(n *net.IPNet) net.IP {
	ip := make(net.IP, len(n.IP))
	copy(ip, n.IP)

	// Calculate mask
	_, bits := n.Mask.Size()
	if bits == 32 { // IPv4
		// Create random bytes
		randBytes := make([]byte, 4)
		rand.Read(randBytes) // global math/rand

		// Result = (IP & Mask) | (Random & ^Mask)
		for i := 0; i < len(ip); i++ {
			ip[i] = (ip[i] & n.Mask[i]) | (randBytes[i] & ^n.Mask[i])
		}
	}
	// TODO: IPv6 support (bits == 128)
	return ip
}
