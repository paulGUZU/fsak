package banner

import (
	"fmt"
	"strings"
	"time"

	"github.com/fatih/color"
)

func Print(role string) {
	// Simple ASCII Art "FSAK"
	// F     S    A    K
	// F---  \    /-\  |/
	// |     _/_ /   \ |\
	
	// A bit more elaborate ASCII
	art := `
███████╗███████╗ █████╗ ██╗  ██╗
██╔════╝██╔════╝██╔══██╗██║ ██╔╝
█████╗  ███████╗███████║█████╔╝ 
██╔══╝  ╚════██║██╔══██║██╔═██╗ 
██║     ███████║██║  ██║██║  ██╗
╚═╝     ╚══════╝╚═╝  ╚═╝╚═╝  ╚═╝
`
	c := color.New(color.FgCyan, color.Bold)
	c.Println(art)
	
	fmt.Printf("   %s :: High Performance SOCKS5 Proxy\n", role)
	fmt.Printf("   Start Time: %s\n", time.Now().Format(time.RFC1123))
	fmt.Println(strings.Repeat("-", 50))
}

func PrintClientStatus(proxyPort int, serverAddr string, tls bool) {
	color.Green("✓ Client Started Successfully")
	fmt.Printf("   • Mode:        Client\n")
	fmt.Printf("   • Listening:   127.0.0.1:%d (SOCKS5)\n", proxyPort)
	fmt.Printf("   • Server Target: %s\n", serverAddr)
	status := "Plaintext"
	if tls {
		status = "TLS/Secure"
	}
	fmt.Printf("   • Transport:   %s\n", status)
	fmt.Println(strings.Repeat("-", 50))
}

func PrintServerStatus(listenAddr string) {
	color.Green("✓ Server Started Successfully")
	fmt.Printf("   • Mode:        Server\n")
	fmt.Printf("   • Listening:   %s\n", listenAddr)
	fmt.Println(strings.Repeat("-", 50))
}
