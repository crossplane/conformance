# conformance

A [Sonobuoy] plugin to assess [Crossplane] conformance. To try it, first
download the `sonobuoy` CLI, then:

```console
# To determine whether a Crossplane distribution is conformant. The distribution
# must be pre-installed on the cluster where Sonobuoy will run.
sonobuoy run --wait --plugin https://raw.githubusercontent.com/crossplane/conformance/main/plugin-crossplane.yaml
sonobuoy results $(sonobuoy retrieve) -m dump

# To determine whether a Crossplane provider is conformant. The provider must be
# pre-installed on the cluster where Sonobuoy will run, and must be the only
# provider installed on the cluster.
sonobuoy run --wait --plugin https://raw.githubusercontent.com/crossplane/conformance/main/plugin-provider.yaml
sonobuoy results $(sonobuoy retrieve) -m dump
```

Note that the provider conformance tests require some advanced setup. The test
requires that at least one of each kind of managed resource exists and is ready
(in the status condition sense).

[sonobuoy]: https://sonobuoy.io/
[crossplane]: https://crossplane.io/
