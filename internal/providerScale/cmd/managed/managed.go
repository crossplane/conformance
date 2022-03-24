// Copyright 2022 The Crossplane Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package managed

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/crossplane/conformance/internal/providerScale/cmd/common"
)

// RunExperiment runs the experiment according to command-line inputs.
// Firstly the input manifests are deployed. After the all MRs are ready, time to readiness metrics are calculated.
// Then, by default, all deployed MRs are deleted.
func RunExperiment(mrTemplatePaths map[string]int, clean bool) ([]*common.Result, error) { //nolint:gocyclo
	var timeToReadinessResults []*common.Result //nolint:prealloc

	for mrPath, count := range mrTemplatePaths {
		for i := 1; i <= count; i++ {
			cmd, err := exec.Command("./internal/providerScale/cmd/managed/manage-mr.sh", "apply", mrPath, strconv.Itoa(i)).Output() //nolint:gosec
			fmt.Print(string(cmd))
			if err != nil {
				return nil, err
			}
		}
	}
	for mrPath, count := range mrTemplatePaths {
		checkReadiness(mrPath, count)
	}
	for mrPath, count := range mrTemplatePaths {
		timeToReadinessResult, err := calculateReadinessDuration(mrPath, count)
		if err != nil {
			return nil, err
		}
		timeToReadinessResults = append(timeToReadinessResults, timeToReadinessResult)
	}
	if clean {
		for mrPath, count := range mrTemplatePaths {
			fmt.Println("Deleting resources...")
			for i := 1; i <= count; i++ {
				cmd, err := exec.Command("./internal/providerScale/cmd/managed/manage-mr.sh", "delete", mrPath, strconv.Itoa(i)).Output() //nolint:gosec
				fmt.Print(string(cmd))
				if err != nil {
					return nil, err
				}
			}
		}
		for mrPath, count := range mrTemplatePaths {
			i := 1
			for i <= count {
				fmt.Println("Checking deletion of resources...")
				cmd, err := exec.Command("./internal/providerScale/cmd/managed/checkDeletion.sh", mrPath, strconv.Itoa(i)).Output() //nolint:gosec
				if err != nil {
					return nil, err
				}
				if len(cmd) != 0 {
					time.Sleep(10 * time.Second)
					continue
				}
				i++
			}
		}
	}
	return timeToReadinessResults, nil
}

func checkReadiness(mrPath string, count int) {
	i := 1
	for i <= count {
		fmt.Println("Checking readiness of resources...")
		isReady, _ := exec.Command("./internal/providerScale/cmd/managed/checkReadiness.sh", mrPath, strconv.Itoa(i)).Output() //nolint:gosec
		if !strings.Contains(string(isReady), "True") {
			time.Sleep(10 * time.Second)
			continue
		}
		i++
	}
}

func calculateReadinessDuration(mrPath string, count int) (*common.Result, error) {
	result := &common.Result{}
	for i := 1; i <= count; i++ {
		fmt.Println("Calculating readiness durations of resources...")
		creationTimeByte, err := exec.Command("./internal/providerScale/cmd/managed/getCreationTime.sh", mrPath, strconv.Itoa(i)).Output() //nolint:gosec
		if err != nil {
			return nil, err
		}
		readinessTimeByte, err := exec.Command("./internal/providerScale/cmd/managed/getReadinessTime.sh", mrPath, strconv.Itoa(i)).Output() //nolint:gosec
		if err != nil {
			return nil, err
		}
		creationTimeString := string(creationTimeByte)
		creationTimeString = creationTimeString[:strings.Index(creationTimeString, `Z`)+1]
		creationTime, err := time.Parse(time.RFC3339, creationTimeString)
		if err != nil {
			return nil, err
		}
		readinessTimeString := string(readinessTimeByte)
		readinessTimeString = readinessTimeString[strings.Index(readinessTimeString, `"`)+1 : strings.Index(readinessTimeString, `Z`)+1]
		readinessTime, err := time.Parse(time.RFC3339, readinessTimeString)
		if err != nil {
			return nil, err
		}
		diff := readinessTime.Sub(creationTime)
		result.Data = append(result.Data, common.Data{Value: diff.Seconds()})
	}
	result.Metric = mrPath
	result.Average, result.Peak = common.CalculateAverageAndPeak(result.Data)
	return result, nil
}
