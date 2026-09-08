package model

type SystemInfo struct {
	ID         string `json:"id"`
	Hostname   string `json:"hostname"`
	OSInfo     OSInfo `json:"os_info"`
	IPAddress  string `json:"ip_address"`
	MACAddress string `json:"mac_address"`
}

type OSInfo struct {
	Name    string `json:"name"`
	Edition string `json:"edition"`
	Version string `json:"version"`
}
