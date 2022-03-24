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

package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/crossplane/conformance/internal/providerScale/cmd/common"
	"github.com/crossplane/conformance/internal/providerScale/cmd/managed"

	"github.com/prometheus/common/model"

	"github.com/spf13/cobra"
)

// QuantifyOptions represents the options of quantify command
type QuantifyOptions struct {
	providerPod       string
	providerNamespace string
	mrPaths           map[string]int
	cmd               *cobra.Command
	address           string
	startTime         time.Time
	endTime           time.Time
	stepDuration      time.Duration
	clean             bool
}

// NewCmdQuantify creates a cobra command
func NewCmdQuantify() *cobra.Command {
	o := QuantifyOptions{}
	o.cmd = &cobra.Command{
		Use: "provider-scale [flags]",
		Short: "This tool collects CPU & Memory Utilization and time to readiness of MRs metrics of providers and " +
			"reports them. When you execute this tool an end-to-end experiment will run.",
		Example: "provider-scale --mrs ./internal/providerScale/manifests/virtualnetwork.yaml=2 " +
			"--mrs ./internal/providerScale/manifests/loadbalancer.yaml=2" +
			"--provider-pod crossplane-provider-jet-azure " +
			"--provider-namespace crossplane-system",
		RunE: o.Run,
	}

	o.cmd.Flags().StringVar(&o.providerPod, "provider-pod", "", "Pod name of provider")
	o.cmd.Flags().StringVar(&o.providerNamespace, "provider-namespace", "crossplane-system",
		"Namespace name of provider")
	o.cmd.Flags().StringToIntVar(&o.mrPaths, "mrs", nil, "Managed resource templates that will be deployed")
	o.cmd.Flags().StringVar(&o.address, "address", "http://localhost:9090", "Address of Prometheus service")
	o.cmd.Flags().DurationVar(&o.stepDuration, "step-duration", 30*time.Second, "Step duration between two data points")
	o.cmd.Flags().BoolVar(&o.clean, "clean", true, "Delete deployed MRs")

	if err := o.cmd.MarkFlagRequired("provider-pod"); err != nil {
		panic(err)
	}
	if err := o.cmd.MarkFlagRequired("mrs"); err != nil {
		panic(err)
	}

	return o.cmd
}

// Run executes the quantify command's tasks.
func (o *QuantifyOptions) Run(_ *cobra.Command, _ []string) error {
	o.startTime = time.Now()
	fmt.Printf("Experiment Started %v\n\n", o.startTime)
	timeToReadinessResults, err := managed.RunExperiment(o.mrPaths, o.clean)
	if err != nil {
		return err
	}
	o.endTime = time.Now()
	fmt.Printf("\nExperiment Ended %v\n\n", o.endTime)
	fmt.Printf("Results\n------------------------------------------------------------\n")
	fmt.Printf("Experiment Duration: %f seconds\n", o.endTime.Sub(o.startTime).Seconds())
	queryResultCPU, err := o.CollectData(fmt.Sprintf(`sum(node_namespace_pod_container:container_cpu_usage_seconds_total:sum_irate{pod="%s", namespace="%s"})*100`,
		o.providerPod, o.providerNamespace))
	if err != nil {
		return err
	}
	cpuResult, err := common.ConstructResult(queryResultCPU)
	if err != nil {
		return err
	}
	queryResultMemory, err := o.CollectData(fmt.Sprintf(`sum(node_namespace_pod_container:container_memory_working_set_bytes{pod="%s", namespace="%s"})`,
		o.providerPod, o.providerNamespace))
	if err != nil {
		return err
	}
	memoryResult, err := common.ConstructResult(queryResultMemory)
	if err != nil {
		return err
	}

	for _, timeToReadinessResult := range timeToReadinessResults {
		fmt.Printf("Average Time to Readiness of %s: %f \n", timeToReadinessResult.Metric, timeToReadinessResult.Average)
		fmt.Printf("Peak Time to Readiness of %s: %f \n", timeToReadinessResult.Metric, timeToReadinessResult.Peak)
	}
	fmt.Printf("Average CPU: %f seconds \n", cpuResult.Average)
	fmt.Printf("Peak CPU: %f seconds \n", cpuResult.Peak)
	fmt.Printf("Average Memory: %f Bytes \n", memoryResult.Average)
	fmt.Printf("Peak Memory: %f Bytes \n", memoryResult.Peak)
	return nil
}

// CollectData sends query and collect data by using the prometheus client
func (o *QuantifyOptions) CollectData(query string) (model.Value, error) {
	client := common.ConstructPrometheusClient(o.address)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	r := common.ConstructTimeRange(o.startTime, o.endTime, o.stepDuration)
	result, warnings, err := client.QueryRange(ctx, query, r)
	if err != nil {
		return nil, err
	}
	if len(warnings) > 0 {
		fmt.Printf("Warnings: %v\n", warnings)
	}
	return result, err
}
