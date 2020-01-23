This directory mirrors the source code via symlinks.
This makes it possible to vendor v2.x releases of
external-snapshotter with `dep` versions that do not
support semantic imports. Support for that is currently
[pending in dep](https://github.com/golang/dep/pull/1963).

If users of dep have enabled pruning, they must disable if
for external-snapshotter in their Gopk.toml, like this:

```toml
[prune]
  go-tests = true
  unused-packages = true

  [[prune.project]]
    name = "github.com/kubernetes-csi/external-snapshotter"
    unused-packages = false
```
