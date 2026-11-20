package local

import (
	"fmt"
	"net"
	"net/netip"
	"os"
	"strconv"
	"strings"
)

const (
	DefaultAddress       = "127.0.0.1:15000"
	DefaultListenAddress = "0.0.0.0:15000"
)

func Address(fallback string) string {
	if address := os.Getenv("HARNESS_ADDR"); address != "" {
		return address
	}
	return fallback
}

func IsLoopback(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip, err := netip.ParseAddr(host)
	return err == nil && ip.Unmap().IsLoopback()
}

func ValidateAddress(address string) error {
	host, port, splitErr := net.SplitHostPort(address)
	number, portErr := strconv.ParseUint(port, 10, 16)
	if splitErr != nil || !IsLoopback(host) || portErr != nil || number == 0 {
		return fmt.Errorf("address must be a loopback host and numeric port, such as %s", DefaultAddress)
	}
	return nil
}
