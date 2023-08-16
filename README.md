# IPFS DNSLink Pinner

A background worker that regularly checks DNSLink pointers to IPFS (or IPNS) content and ensures the local/target IPFS node has (only) that content pinned.

## Usage

```sh
$ tree ./domains
./domains
├── .dnslink-pinner.toml
├── ipfs.io
├── cid.ipfs.tech
└── explore.ipld.io

$ dnslink-pinner ./domains --api /ip4/127.0.0.1/tcp/5001
2023/08/16 07:59:43 INFO Checking ipfs.io
2023/08/16 07:59:43 INFO Checking cid.ipfs.tech
2023/08/16 07:59:43 INFO Checking explore.ipld.io
```
