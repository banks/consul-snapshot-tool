# Consul Snapshot Inspection Tool

This is a quick and dirty tool for reading in a [Consul](https://www.consul.io) snapshot file from the raft directory and dumping some statistics about which types of data are consuming the space.

It's fairly basic and quick and can certainly be made easier to use - PRs welcome. We may consider merging this into a more official tool or even the Consul binary if it prooves useful.

## Building

If you have a working Go toolchain you should be able to install this with:

```
$ go get -u github.com/banks/consul-snapshot-tool
```

If you want to cross compile it for Linux from another OS (e.g. so you can run the tool on a server where the snapshot file is without moving it or installing Go on the server):

 1. Checkout this repo into your `$GOPATH`. (Go modules may also work with go 1.12+, not tried yet).
 2. Compile with `GOOS=linux go build .`. Assuming your server has same CPU architecture as the server - if not checkout another resource on cross-compiling Go, it's not hard!
 3. Copy the `consul-snapshot-tool` binary to the linux server and run it there.

 ## Usage

 There is only one way to use this an no options currently. It reads from STDIN so:

 ```sh
 $ cat /tmp/consul/raft/sna....32/state.bin | consul-snapshot-tool
           Record Type    Count   Total Size
---------------------- -------- ------------
                   KVS     4294      489.7KB
              Register      104         57KB
             Tombstone      530       55.4KB
               Session       26        4.5KB
                 Index        9         220B
             Autopilot        1         188B
 CoordinateBatchUpdate        1         167B
---------------------- -------- ------------
                         TOTAL:      607.2KB
 ```

 ### Backup Snapshots

 To inspect a snapshot made using `consul snapshot save` you first need to extract the raw snapshot file. The snapshot is actually a zipped tar archive of the snapshot and some metadata.

 ```sh
 $ tar -xzf backup.snap
 $ cat state.bin | consul-snapshot-tool
            Record Type    Count   Total Size
---------------------- -------- ------------
                   KVS     4461      508.8KB
              Register      104         57KB
                 Index        9         220B
             Autopilot        1         188B
 CoordinateBatchUpdate        1         167B
---------------------- -------- ------------
                         TOTAL:      566.3KB
```
