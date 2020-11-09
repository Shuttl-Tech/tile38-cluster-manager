# Tile38 Cluster Manager

Tile38 Cluster Manager is a sidecar utility to help deploy a distributed, dynamic [Tile38][] cluster inside Consul Connect network mesh.

The cluster manager runs as a proxy alongside a Tile38 database and provides following features:

 - Clustering with leader election
 - Data replication from active leader
 - Handling node failures and promoting a follower
 - Authorization and network traffic encryption using Consul Connect

## Install

Pre-built binaries are available from [releases][] page. Alternatively, you can install the manager using `go get` or pull the docker image.

Using `go get`

```sh
go get -u github.com/Shuttl-Tech/tile38-cluster-manager
```

[releases]: https://github.com/Shuttl-Tech/tile38-cluster-manager/releases

For more details please check out [the documentation][].


## License

Source code, binaries and docker images are available under [MIT License][]

[the documentation]: https://shuttl-tech.github.io/tile38-cluster-manager
[Tile38]: https://github.com/tidwall/tile38
[MIT License]: ./LICENSE