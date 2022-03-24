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
		Use:     "Quantify the performance metrics",
		Short:   "quantify",
		Example: "Example",
		RunE:    o.Run,
	}

	o.cmd.Flags().StringVar(&o.providerPod, "provider-pod", "", "provider pod name...")
	o.cmd.Flags().StringVar(&o.providerNamespace, "provider-namespace", "crossplane-system", "provider namespace...")
	o.cmd.Flags().StringToIntVar(&o.mrPaths, "mrs", map[string]int{}, "mr paths...")
	o.cmd.Flags().StringVar(&o.address, "address", "http://localhost:9090", "address...")
	o.cmd.Flags().DurationVar(&o.stepDuration, "step-duration", 30*time.Second, "step duration...")
	o.cmd.Flags().BoolVar(&o.clean, "clean", true, "clean cluster...")

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
	fmt.Printf("Average CPU: %f %% \n", cpuResult.Average)
	fmt.Printf("Peak CPU: %f %% \n", cpuResult.Peak)
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
