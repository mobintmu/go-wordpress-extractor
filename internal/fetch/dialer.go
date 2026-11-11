package fetch

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"time"
)

// Resolver returns the IP addresses for host.
type Resolver func(ctx context.Context, host string) ([]net.IP, error)

// SystemResolver uses the system DNS resolver.
func SystemResolver(ctx context.Context, host string) ([]net.IP, error) {
	addrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	ips := make([]net.IP, 0, len(addrs))
	for _, a := range addrs {
		ips = append(ips, a.IP)
	}
	return ips, nil
}

// NotAllowedError means the destination is a private address or a
// blocked port.
type NotAllowedError struct {
	Host   string
	Port   int
	IP     net.IP
	Reason string
}

func (e *NotAllowedError) Error() string {
	if e.IP != nil {
		return fmt.Sprintf("%s resolved to %s, which is %s", e.Host, e.IP, e.Reason)
	}
	return fmt.Sprintf("%s:%d is not allowed: %s", e.Host, e.Port, e.Reason)
}

// BlockedPorts are ports of common internal services.
var BlockedPorts = map[int]bool{
	0: true, 19: true, 21: true, 22: true, 23: true, 25: true, 53: true, 110: true, 111: true, 135: true,
	137: true, 139: true, 143: true, 389: true, 445: true, 465: true, 587: true, 636: true, 993: true,
	995: true, 1433: true, 1521: true, 2049: true, 2375: true, 2376: true, 2379: true, 2380: true,
	3306: true, 3389: true, 5432: true, 5672: true, 5984: true, 6379: true, 6443: true, 8500: true,
	9000: true, 9042: true, 9092: true, 9200: true, 9300: true, 10250: true, 11211: true, 15672: true,
	27017: true,
}

var deniedNets = mustCIDRs(
	"0.0.0.0/8", "10.0.0.0/8", "100.64.0.0/10", "127.0.0.0/8", "169.254.0.0/16",
	"172.16.0.0/12", "192.0.0.0/24", "192.0.2.0/24", "192.168.0.0/16", "198.18.0.0/15",
	"198.51.100.0/24", "203.0.113.0/24", "224.0.0.0/4", "240.0.0.0/4",
	"::/128", "::1/128", "64:ff9b::/96", "100::/64", "2001:db8::/32", "fc00::/7", "fe80::/10", "ff00::/8",
)

func mustCIDRs(list ...string) []*net.IPNet {
	out := make([]*net.IPNet, 0, len(list))
	for _, c := range list {
		_, n, err := net.ParseCIDR(c)
		if err != nil {
			panic(err)
		}
		out = append(out, n)
	}
	return out
}

// SafeDialer resolves the host itself and refuses private, loopback,
// link-local, and metadata addresses after resolution.
type SafeDialer struct {
	Resolve Resolver
	// Allowed lists networks exempt from the deny list (tests only).
	Allowed []*net.IPNet
	dialer  net.Dialer
}

// NewSafeDialer builds a dialer with the given resolver and exemptions.
func NewSafeDialer(resolve Resolver, allowed []*net.IPNet) *SafeDialer {
	if resolve == nil {
		resolve = SystemResolver
	}
	return &SafeDialer{
		Resolve: resolve,
		Allowed: allowed,
		dialer:  net.Dialer{Timeout: 15 * time.Second, KeepAlive: 30 * time.Second},
	}
}

// IPAllowed reports whether ip may be contacted.
func (d *SafeDialer) IPAllowed(ip net.IP) bool {
	if v4 := ip.To4(); v4 != nil {
		ip = v4
	}
	for _, n := range d.Allowed {
		if n.Contains(ip) {
			return true
		}
	}
	for _, n := range deniedNets {
		if n.Contains(ip) {
			return false
		}
	}
	return !ip.IsLoopback() && !ip.IsPrivate() && !ip.IsLinkLocalUnicast() && !ip.IsUnspecified() && !ip.IsMulticast()
}

// DialContext implements the transport dial function.
func (d *SafeDialer) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return nil, err
	}
	if BlockedPorts[port] {
		return nil, &NotAllowedError{Host: host, Port: port, Reason: "a blocked port"}
	}
	var ips []net.IP
	if ip := net.ParseIP(host); ip != nil {
		ips = []net.IP{ip}
	} else {
		ips, err = d.Resolve(ctx, host)
		if err != nil {
			return nil, err
		}
	}
	if len(ips) == 0 {
		return nil, &net.DNSError{Err: "no addresses", Name: host, IsNotFound: true}
	}
	var lastErr error
	for _, ip := range ips {
		if !d.IPAllowed(ip) {
			lastErr = &NotAllowedError{Host: host, Port: port, IP: ip, Reason: "a private, loopback, link-local, or reserved address"}
			continue
		}
		conn, err := d.dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), portStr))
		if err == nil {
			return conn, nil
		}
		lastErr = err
	}
	return nil, lastErr
}

// CheckPort reports a NotAllowedError for a blocked port before any I/O.
func CheckPort(host string, port int) error {
	if BlockedPorts[port] {
		return &NotAllowedError{Host: host, Port: port, Reason: "a blocked port"}
	}
	return nil
}
