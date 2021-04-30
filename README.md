# conformance

An experimental [Sonobuoy] plugin to assess [Crossplane] conformance. To try it,
first download the `sonobuoy` CLI, then:

```console
sonobuoy run --wait --plugin https://raw.githubusercontent.com/crossplane/conformance/main/plugin.yaml
sonobuoy results $(sonobuoy retrieve) -m dump
```

[sonobuoy]: https://sonobuoy.io/
[crossplane]: https://crossplane.io/
