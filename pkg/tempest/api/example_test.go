package api_test

import (
	"fmt"
	"time"

	"github.com/lennrt/tempestkeep/pkg/tempest/api"
)

func ExampleNew() {
	client, err := api.New("synthetic-test-token",
		api.WithCacheTTL(0),
		api.WithRequestTimeout(10*time.Second),
	)
	fmt.Println(client != nil, err)
	// Output: true <nil>
}

func ExamplePickTempestDevice() {
	stations := []api.Station{{
		StationID: 7,
		Devices: []api.Device{
			{DeviceID: 11, DeviceType: "AR"},
			{DeviceID: 12, DeviceType: "ST"},
		},
	}}
	station, deviceID, ok := api.PickTempestDevice(stations)
	fmt.Println(station.StationID, deviceID, ok)
	// Output: 7 12 true
}
