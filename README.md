# Crossplane Conformance Suite

A [Sonobuoy] plugin to assess [Crossplane] conformance. To try it, first
download the `sonobuoy` CLI, then:

```console
# The version of Crossplane you wish to conform with.
CROSSPLANE_VERSION=1.2

# To determine whether a Crossplane distribution is conformant. The distribution
# must be pre-installed on the cluster where Sonobuoy will run.
sonobuoy run --wait --plugin https://raw.githubusercontent.com/crossplane/conformance/release-${CROSSPLANE_VERSION}/plugin-crossplane.yaml
sonobuoy results $(sonobuoy retrieve) -m dump

# To determine whether a Crossplane provider is conformant. The provider must be
# pre-installed on the cluster where Sonobuoy will run, and must be the only
# provider installed on the cluster.
sonobuoy run --wait --plugin https://raw.githubusercontent.com/crossplane/conformance/release-${CROSSPLANE_VERSION}/plugin-provider.yaml
sonobuoy results $(sonobuoy retrieve) -m dump
```

> Note that the provider conformance tests require some advanced setup. The test
> requires that at least one of each kind of managed resource exists and is
> ready (in the status condition sense).

This repository maintains a branch for each version of Crossplane that the
conformance test suite supports. For example branch `release-1.2` will test
conformance with Crossplane 1.2.x. The docker image that powers the conformance
suite also derives its version from the version of Crossplane it corresponds to.
Tags are of the form `1.2.0-cf.1`, where `1.2.x` is the target version of
Crossplane, and `cf.1` is the version of the conformance test.

Note that the conformance tests are _not_ typically sensitive to Crossplane
patch releases. That is, in most cases when Crossplane `1.2.1` is released the
conformance test suite's version will not be bumped accordingly, and will remain
at `1.2.0-cf.X`. In the unlikely event that a Crossplane patch release
necessitates a conformance test suite update the patch version of the test suite
will be updated to match the Crossplane patch version. The `image` referenced by
the [distribution] and [provider] plugins in any particular release branch is
always the authoritative conformance test for that major and minor version of
Crossplane.

[sonobuoy]: https://sonobuoy.io/
[crossplane]: https://crossplane.io/
[distribution]: ./plugin-crossplane.yaml
[provider]: ./plugin-provider.yaml
