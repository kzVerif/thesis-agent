package service

import "net"

func getPrimaryNetwork() (string, string) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return "", ""
	}

	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok || ipNet.IP.To4() == nil {
				continue
			}
			return ipNet.IP.String(), iface.HardwareAddr.String()
		}
	}

	return "", ""
}
