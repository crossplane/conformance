package common

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/prometheus/common/model"

	"github.com/prometheus/client_golang/api"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
)

type Data struct {
	Timestamp time.Time
	Value     float64
}

type Result struct {
	Data          []Data
	Metric        string
	Peak, Average float64
}

func ConstructPrometheusClient(address string) v1.API {
	client, err := api.NewClient(api.Config{
		Address: address,
	})

	if err != nil {
		fmt.Printf("Error creating client: %v\n", err)
		os.Exit(1)
	}

	return v1.NewAPI(client)
}

func ConstructTimeRange(startTime, endTime time.Time, stepDuration time.Duration) v1.Range {
	return v1.Range{
		Start: startTime,
		End:   endTime,
		Step:  stepDuration,
	}
}

func ConstructResult(value model.Value) (*Result, error) {
	result := &Result{}
	matrix := value.(model.Matrix)

	for _, m := range matrix {
		result.Metric = m.Metric.String()

		for _, v := range m.Values {
			valueNum, err := strconv.ParseFloat(v.Value.String(), 64)
			if err != nil {
				return nil, err
			}
			result.Data = append(result.Data, Data{Timestamp: v.Timestamp.Time(), Value: valueNum})
		}
	}

	result.Average, result.Peak = CalculateAverageAndPeak(result.Data)
	return result, nil
}

func CalculateAverageAndPeak(data []Data) (float64, float64) {
	var sum, peak float64
	for _, d := range data {
		sum += d.Value

		if d.Value > peak {
			peak = d.Value
		}
	}
	return sum / float64(len(data)), peak
}
