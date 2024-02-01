# IPFS DNSLink Pinner

A background worker that regularly checks DNSLink pointers to IPFS (or IPNS) content and ensures the local/target IPFS node has that content pinned.

## Install

```sh
go install github.com/jphastings/dnslink-pinner@latest
```

## Usage

```sh
$ tree ./test-domains
./test-domains
├── cid.ipfs.tech
├── explore.ipld.io
├── ipfs.tech
└── www.byjp.me

$ dnslink-pinner ./test-domains --api /ip4/127.0.0.1/tcp/5001
2024/02/01 12:48:11 ➕ cid.ipfs.tech
2024/02/01 12:48:11 ➕ explore.ipld.io
2024/02/01 12:48:11 ➕ ipfs.tech
2024/02/01 12:48:11 ➕ www.byjp.me
2024/02/01 12:48:16 📍 QmNfgDHX3Yt3aXkzH5NHbKXohonNDS49seFsUBooVF4HDh (cid.ipfs.tech)
2024/02/01 12:48:22 📍 Qmdeo8xnLCFpGrHooSussnDG2JLSBkkpfpAoSXQZodXpRU (www.byjp.me)
2024/02/01 12:48:28 📍 QmX4kjZAJehEG33NmZYJjkx4ZkRcA5JYWQ5tHXZaPdmhHx (explore.ipld.io)
2024/02/01 12:48:35 📍 QmUZPi7DaFHEdvitvDcuH5AMTLJHskEdmQVKv89aQ3FckU (ipfs.tech)
```

You can add, remove, and request a re-check of domains while the pinner is running:

```sh
# Request refresh of a domain by touching
$ touch ./test-domains/cid.ipfs.tech
# Remove a domain by removing
$ rm ./test-domains/cid.ipfs.tech
# Add a domain by creating
$ touch ./test-domains/cid.ipfs.tech
```
