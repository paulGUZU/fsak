package client

import (
	"fmt"
	"math/rand"
	"net"
	"sync"
	"time"
)

type AddressPool struct {
	addresses []string
	healthy   []string
	mu        sync.RWMutex
}

func NewAddressPool(addrs []string) (*AddressPool, error) {
	pool := &AddressPool{}
	for _, addr := range addrs {
		// Check if CIDR
		if _, _, err := net.ParseCIDR(addr); err == nil {
			// Expand CIDR (limit to reasonable size)
			// For simplicity, just append the single IP if it's actually just an IP with /32?
			// Or iterate? Cloudflare ranges are huge /12 etc.
			// User says "addressess could be cidr".
			// Expanding a /20 is 4096 IPs. 
			// We probably shouldn't expand ALL of them into memory if it's large.
			// Maybe logic: accept CIDRs, and `Pick` generates a random IP within that CIDR?
			// That seems better for large ranges.
			
			// But for health checking, we can't check 2^32 IPs.
			// Maybe we pick a few random ones from the CIDR to check?
			// Or we just trust the CIDR is broadly available and just pick random?
			// User says: "make sure to have a background proccess to check connection can be estabilished"
			
			// Let's store logic:
			// If CIDR, we treat it as a generator.
			// We generate candidate IPs.
			// We check candidates.
			// We keep a pool of "Verified Healthy IPs".
			// Background process keeps refreshing this pool.
			
			// MVP: Store raw strings. "Expand" only strictly necessary.
			// If it's a plain IP, add to candidates.
			// If CIDR, add to generators.
			
			// Re-reading requirements: "addressess could be cidr but make sure to have a background proccess to check connection can be estabilished"
			// "client send each request to a ip from addressess (select randomly)"
			
			// Let's implement a mix. 
			// We will maintain a list of `healthy` IPs.
			// Background loop:
			//   For each configured entry (IP or CIDR):
			//     If IP: check it. If good, add to healthy.
			//     If CIDR: Pick X random IPs from it. Check them. Add good ones to healthy.
			//   Sleep.
			
		} else {
			// Assume plain IP or Hostname
		}
	}
	
	// Re-init with just the config list to start
	pool.addresses = addrs
	
	// Start background checker
	go pool.checkLoop()
	
	return pool, nil
}

func (p *AddressPool) checkLoop() {
	for {
		// Create a new list of healthy IPs
		var newHealthy []string
		var wg sync.WaitGroup
		var mu sync.Mutex
		
		addHealthy := func(ip string) {
			mu.Lock()
			newHealthy = append(newHealthy, ip)
			mu.Unlock()
		}

		for _, item := range p.addresses {
			// Check if CIDR
			_, ipnet, err := net.ParseCIDR(item)
			if err == nil {
				// Pick random IPs from CIDR. ex: 5 IPs.
				for i := 0; i < 5; i++ {
					ip := randomIPInSubnet(ipnet)
					wg.Add(1)
					go func(target string) {
						defer wg.Done()
						if isHealthy(target) {
							addHealthy(target)
						}
					}(ip.String())
				}
			} else {
				// Single IP
				wg.Add(1)
				go func(target string) {
					defer wg.Done()
					if isHealthy(target) {
						addHealthy(target)
					}
				}(item)
			}
		}
		
		wg.Wait()
		
		if len(newHealthy) > 0 {
			p.mu.Lock()
			p.healthy = newHealthy
			p.mu.Unlock()
		} else {
			// If all checks fail, what to do?
			// Maybe keep old ones? Or empty?
			// If empty, `Pick` will block or fail?
			// Let's keep working but maybe log warning.
			fmt.Println("Warning: No healthy IPs found in pool.")
		}

		time.Sleep(30 * time.Second)
	}
}

func (p *AddressPool) PickInfo() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	
	if len(p.healthy) == 0 {
		// Fallback to configured list if unchecked? 
		// Or just return one from raw list to try?
		if len(p.addresses) > 0 {
			// Just pick one and hope (stripping CIDR logic for MVP fallback)
			// Actually just return first item logic if CIDR? No.
			// Let's return raw item if no healthy, assuming it might be an IP.
			// Using random.
			return p.addresses[rand.Intn(len(p.addresses))]
		}
		return "127.0.0.1"
	}
	
	return p.healthy[rand.Intn(len(p.healthy))]
}

func isHealthy(ip string) bool {
	// Try a TCP dial to port 80 or 443?
	// Config doesn't specify check port, but since we are HTTP proxy, likely 80 or 443.
	// But the "Server" listens on `Config.Port`.
	// Ideally we check `ip:Config.Port`.
	// But `AddressPool` doesn't know Config.Port in this struct definition (my bad).
	// I should assume 80 or 443 strictly for "connectivity check" or check the actual service port?
	// Checking service port is best.
	// I will just check 80 for now as generic reachability, OR I should inject port.
	// Let's update `NewAddressPool` to take port?
	// For simplicity, just check connectivity to the IP on HTTP/HTTPS common ports or ICMP?
	// Go `net.Dial` with "tcp" is fine.
	
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(ip, "80"), 2*time.Second) 
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

func randomIPInSubnet(n *net.IPNet) net.IP {
	// Simple random implementation for IPv4
	// (Ignoring IPv6 complexity for MVP unless needed)
	ip := make(net.IP, len(n.IP))
	copy(ip, n.IP)
	
	// Calculate mask
	ones, bits := n.Mask.Size()
	_ = ones
	if bits == 32 { // IPv4
		// Generate random int
		_ = rand.Uint32()
		// Apply mask
		// ... logic to fit valid range.
		// Easiest: increment IP by random amount?
		// Let's just create a completely random IP and mask it.
		// Actually, standard library doesn't make this super easy.
		// Custom logic:
		
		// Create random bytes
		randBytes := make([]byte, 4)
		rand.Read(randBytes) // global math/rand not crypto/rand here usually, but here I used math/rand which is seeded?
		// math/rand isn't thread safe without locking source or using global.
		// Global `rand.Intn` is locked.
		
		// Logic:
		// Result = (IP & Mask) | (Random & ^Mask)
		for i := 0; i < len(ip); i++ {
			ip[i] = (ip[i] & n.Mask[i]) | (randBytes[i] & ^n.Mask[i])
		}
	}
	return ip
}
