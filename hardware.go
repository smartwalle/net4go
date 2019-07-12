package net4go

import (
	"errors"
	"net"
)

var (
	ErrFailedToObtainHardwareAddr = errors.New("net4go: failed to obtain hardware address")
)

func GetHardwareAddr() (string, error) {
	netInterfaces, err := net.Interfaces()
	if err != nil {
		return "", err
	}

	for _, netInterface := range netInterfaces {
		var addr = netInterface.HardwareAddr.String()
		if len(addr) != 0 {
			return addr, nil
		}
	}
	return "", ErrFailedToObtainHardwareAddr
}

func GetHardwareAddrs() ([]string, error) {
	netInterfaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	var addrs []string
	for _, netInterface := range netInterfaces {
		var addr = netInterface.HardwareAddr.String()
		if len(addr) != 0 {
			addrs = append(addrs, addr)
		}
	}
	return addrs, nil
}
