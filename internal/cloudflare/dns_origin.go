package cloudflare

import (
	"fmt"
	"net"
)

// GreyCloudRecords builds DNS-only A or AAAA record specs for a single origin IP.
// IPv4 yields one A record; IPv6 yields one AAAA record (not both).
func GreyCloudRecords(originIP string) ([]struct {
	Type    string
	Content string
}, error) {
	ip := net.ParseIP(originIP)
	if ip == nil {
		return nil, fmt.Errorf("invalid origin ip")
	}
	if v4 := ip.To4(); v4 != nil {
		return []struct {
			Type    string
			Content string
		}{{Type: "A", Content: v4.String()}}, nil
	}
	return []struct {
		Type    string
		Content string
	}{{Type: "AAAA", Content: ip.String()}}, nil
}
