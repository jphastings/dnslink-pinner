package monitor

import (
	"context"
	"fmt"
	"io/fs"
	"log"
	"path/filepath"
	"sync"

	"github.com/ipfs/boxo/coreiface/options"
	"github.com/ipfs/boxo/coreiface/path"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/kubo/client/rpc"
)

type Repo struct {
	domains []*domain
	ipfs    *rpc.HttpApi

	pinSetMutex sync.Mutex
	toPin       []cid.Cid
	toUnpin     []cid.Cid
	toSwap      []struct {
		new cid.Cid
		old cid.Cid
	}
}

func New(rootDir string, ipfs *rpc.HttpApi) (*Repo, error) {
	var domains []*domain
	err := filepath.WalkDir(rootDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}

		if d == nil || d.IsDir() || d.Name()[0:1] == "." {
			return nil
		}

		domain, err := newDomain(path)
		if err != nil {
			return fmt.Errorf("couldn't load domain at %s: %w", d.Name(), err)
		}

		domains = append(domains, domain)

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("couldn't load filesystem repo: %w", err)
	}

	return &Repo{
		domains: domains,
		ipfs:    ipfs,
	}, nil
}

func (r *Repo) flagForRotate(ctx context.Context, d *domain, c cid.Cid) {
	if d.currentCid.Equals(c) {
		r.cueAdd(d.currentCid)
		return
	}

	if !d.currentCid.Defined() {
		r.cueAdd(c)
		_ = d.setCid(c)
		log.Printf("Pinned %s for the first time: %s", d.name, c.String())
		return
	}

	r.cueSwap(c, d.currentCid)
	_ = d.setCid(c)
	log.Printf("Swapped pin for %s, now %s", d.name, c.String())
}

func (r *Repo) cueAdd(newCid cid.Cid) {
	r.pinSetMutex.Lock()
	defer r.pinSetMutex.Unlock()

	r.toPin = append(r.toPin, newCid)
}

func (r *Repo) cueSwap(newCid, oldCid cid.Cid) {
	r.pinSetMutex.Lock()
	defer r.pinSetMutex.Unlock()

	r.toSwap = append(r.toSwap, struct {
		new cid.Cid
		old cid.Cid
	}{newCid, oldCid})
}

func (r *Repo) cueRemove(oldCid cid.Cid) {
	r.pinSetMutex.Lock()
	defer r.pinSetMutex.Unlock()

	r.toUnpin = append(r.toUnpin, oldCid)
}

func (r *Repo) performPinChanges(ctx context.Context) {
	r.pinSetMutex.Lock()
	defer r.pinSetMutex.Unlock()

	doNotUnpin := make(map[string]struct{})

	for _, pair := range r.toSwap {
		doNotUnpin[pair.new.String()] = struct{}{}

		if err := r.ipfs.Pin().Add(ctx, path.New(pair.new.String()), options.Pin.Recursive(true)); err != nil {
			doNotUnpin[pair.old.String()] = struct{}{}
			log.Printf("Unable to swap in %s (to replace %s): %v", pair.new.String(), pair.old.String(), err)
			continue
		}

		if err := r.ipfs.Pin().Rm(ctx, path.New(pair.old.String())); err != nil {
			log.Printf("Unable to unpin %s, you should manually remove this: %v", pair.old.String(), err)
		}
	}

	for _, c := range r.toPin {
		doNotUnpin[c.String()] = struct{}{}
		if err := r.ipfs.Pin().Add(ctx, path.New(c.String()), options.Pin.Recursive(true)); err != nil {
			log.Printf("Unable to pin %s, this will be retried on the next check: %v", c.String(), err)
		}
	}

	for _, c := range r.toUnpin {
		if _, ok := doNotUnpin[c.String()]; ok {
			continue
		}

		if err := r.ipfs.Pin().Rm(ctx, path.New(c.String())); err != nil {
			log.Printf("Unable to unpin %s, you should manually remove this: %v", c.String(), err)
		}
	}

	return
}
