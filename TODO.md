# TODO

- [ ] Refactor retry delay system; currently rechecks *everything* at the rate of the most changeable.
- [x] Swap to using github.com/dnslink-std/go/tree/main
- [x] Use github.com/ipfs/kubo/client/rpc to pin & unpin
- [x] Swap to using "add" "remove" and "keep" CID lists, so two different domains pointing to the same CID, then one moving away, doesn't remove a needed pin.
