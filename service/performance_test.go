package service

import (
	"encoding/json"
	"testing"
)

func TestPerformanceInfoJSONSchema(t *testing.T) {
	info := PerformanceInfo{
		CPUUsage:   25.5,
		RAMTotalGB: 16, RAMUsedGB: 8, RAMUsage: 50,
		DiskTotalGB: 500, DiskUsedGB: 200, DiskFreeGB: 300, DiskUsage: 40,
	}

	data, err := json.Marshal(info)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"cpu_usage":25.5,"ram_total_gb":16,"ram_used_gb":8,"ram_usage":50,"disk_total_gb":500,"disk_used_gb":200,"disk_free_gb":300,"disk_usage":40}`
	if string(data) != want {
		t.Fatalf("unexpected JSON\nwant: %s\n got: %s", want, data)
	}
}
