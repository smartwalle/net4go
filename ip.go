package net4go

import (
	"errors"
	"io/ioutil"
	"net"
	"net/http"
	"strings"
)

var (
	ErrFailedToObtainIP = errors.New("net4go: failed to obtain ip address")
)

func GetExternalIP() (string, error) {
	rsp, err := http.Get("http://myexternalip.com/raw")
	if rsp != nil {
		defer rsp.Body.Close()
	}
	if err != nil {
		return "", err
	}
	content, err := ioutil.ReadAll(rsp.Body)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(content)), nil
}

func GetInternalIP() (string, error) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "", err
	}

	for _, addr := range addrs {
		if ipNet, ok := addr.(*net.IPNet); ok && !ipNet.IP.IsLoopback() {
			if ipNet.IP.To4() != nil {
				return ipNet.IP.String(), nil
			}
		}
	}
	return "", ErrFailedToObtainIP
}

func GetInternalIPs() ([]string, error) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil, err
	}

	var ips []string
	for _, addr := range addrs {
		if ipNet, ok := addr.(*net.IPNet); ok && !ipNet.IP.IsLoopback() {
			if ipNet.IP.To4() != nil {
				ips = append(ips, ipNet.IP.String())
			}
		}
	}
	return ips, nil
}

func GetRequestIP(r *http.Request) string {
	var remoteAddr string
	if ip := r.Header.Get("X-Real-Ip"); ip != "" {
		remoteAddr = ip
	} else if ip := r.Header.Get("X-Forwarded-For"); ip != "" {
		remoteAddr = strings.Split(ip, ",")[0]
	} else {
		remoteAddr, _, _ = net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	}
	if remoteAddr == "::1" {
		remoteAddr = "127.0.0.1"
	}
	return remoteAddr
}
