package net4go

import (
	"errors"
	"net"
)

var (
	ErrFailedToObtainHardwareAddr = errors.New("failed to obtain ip address")
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
