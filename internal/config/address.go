package config

import (
	"net"
	"strings"
	"time"
)

func (cfg Config) StorageAdvertiseAddress() string {
	if strings.TrimSpace(cfg.StorageAddress) != "" {
		return strings.TrimSpace(cfg.StorageAddress)
	}
	return storageAddressFromHTTPAddress(cfg.HTTPAddress)
}

func (cfg Config) PendingObjectTTLDuration() time.Duration {
	return parseDurationOr(cfg.PendingObjectTTL, time.Hour)
}

func storageAddressFromHTTPAddress(address string) string {
	address = strings.TrimSpace(address)
	if address == "" || strings.Contains(address, "://") {
		return address
	}
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		if strings.HasPrefix(address, ":") {
			return "127.0.0.1" + address
		}
		return address
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	return net.JoinHostPort(host, port)
}
