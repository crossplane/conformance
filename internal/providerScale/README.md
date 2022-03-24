# provider-scale

This tool collects and reports some performance metrics. The details in the [one-pager].

### Usage

When this tool is executed, an end-to-end experiment will run. The inputs of this experiment can be manipulated by using
the command-line options.

```
Flags:
      --address string              Address of Prometheus service (default "http://localhost:9090")
      --clean                       Delete deployed MRs (default true)
  -h, --help                        help for provider-scale
      --mrs stringToInt             Managed resource templates that will be deployed (default [])
      --provider-namespace string   Namespace name of provider (default "crossplane-system")
      --provider-pod string         Pod name of provider
      --step-duration duration      Step duration between two data points (default 30s)
```

The `provider-pod` and `mrs` options are required.

Example usage:

```
provider-scale --mrs ./internal/providerScale/manifests/virtualnetwork.yaml=2
--mrs ./internal/providerScale/manifests/loadbalancer.yaml=2
--provider-pod crossplane-provider-jet-azure
--provider-namespace crossplane-system
```

With this input, two virtualnetwork & loadbalancer MRs will be deployed to the cluster.

**_Note: In template manifests the name must be: `test-{{SUFFIX}}`_**

[one-pager]: https://github.com/crossplane/crossplane/pull/2983