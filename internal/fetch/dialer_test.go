package fetch

import (
	"context"
	"errors"
	"net"
	"testing"
)

func TestIPAllowed(t *testing.T) {
	d := NewSafeDialer(nil, nil)
	for ip, want := range map[string]bool{
		"93.184.216.34": true, "2606:4700::1111": true,
		"127.0.0.1": false, "10.1.2.3": false, "172.16.0.1": false, "192.168.1.1": false,
		"169.254.169.254": false, "100.64.0.1": false, "0.0.0.0": false, "224.0.0.1": false,
		"::1": false, "fe80::1": false, "fd00::1": false, "::ffff:10.0.0.1": false, "::ffff:127.0.0.1": false,
	} {
		if got := d.IPAllowed(net.ParseIP(ip)); got != want {
			t.Errorf("IPAllowed(%s) = %v, want %v", ip, got, want)
		}
	}
	_, lo, _ := net.ParseCIDR("127.0.0.0/8")
	if !NewSafeDialer(nil, []*net.IPNet{lo}).IPAllowed(net.ParseIP("127.0.0.1")) {
		t.Error("exempt network should be allowed")
	}
}

func TestDialRejectsAfterResolution(t *testing.T) {
	d := NewSafeDialer(func(context.Context, string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("169.254.169.254")}, nil
	}, nil)
	_, err := d.DialContext(context.Background(), "tcp", "metadata.example:80")
	var na *NotAllowedError
	if !errors.As(err, &na) || na.IP.String() != "169.254.169.254" {
		t.Fatalf("want NotAllowedError, got %v", err)
	}
	if _, err := d.DialContext(context.Background(), "tcp", "example.com:22"); !errors.As(err, &na) {
		t.Fatalf("blocked port: %v", err)
	}
}
