package model_test

import (
	"fmt"

	"github.com/lennrt/tempestkeep/pkg/tempest/model"
)

func ExampleDeviceObsFromRow() {
	epoch := float64(1_700_000_000)
	temperature := 20.5
	row := make([]*float64, model.DeviceObsFields)
	row[0] = &epoch
	row[7] = &temperature

	observation, err := model.DeviceObsFromRow(row)
	fmt.Println(observation.Epoch, *observation.AirTempC, err)
	// Output: 1700000000 20.5 <nil>
}
