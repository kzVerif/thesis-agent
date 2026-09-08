package model

import (
	"encoding/json"
	"testing"
)

func TestSystemInfoJSONSchema(t *testing.T) {
	info := SystemInfo{
		ID: "86dbfbcc-a13c-4ffc-adab-7187018273e0", Hostname: "kanghunz",
		OSInfo:    OSInfo{Name: "windows", Edition: "amd64", Version: "11"},
		IPAddress: "192.168.1.136", MACAddress: "90:e8:68:d0:16:f9",
	}

	data, err := json.Marshal(info)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"id":"86dbfbcc-a13c-4ffc-adab-7187018273e0","hostname":"kanghunz","os_info":{"name":"windows","edition":"amd64","version":"11"},"ip_address":"192.168.1.136","mac_address":"90:e8:68:d0:16:f9"}`
	if string(data) != want {
		t.Fatalf("unexpected JSON\nwant: %s\n got: %s", want, data)
	}
}
