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

type cidPair struct {
	new cid.Cid
	old cid.Cid
}

type Repo struct {
	domains []*domain
	ipfs    *rpc.HttpApi

	pinSetMutex    sync.Mutex
	domainSetMutex sync.Mutex
	toKeep         []cid.Cid
	toPin          []cid.Cid
	toUnpin        []cid.Cid
	toSwap         []struct {
		new cid.Cid
		old cid.Cid
	}
}

func New(rootDir string, ipfs *rpc.HttpApi) (*Repo, error) {
	repo := &Repo{ipfs: ipfs}
	err := filepath.WalkDir(rootDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}

		if d == nil || d.IsDir() || d.Name()[0:1] == "." {
			return nil
		}

		return repo.AddDomainByPath(path)
	})
	if err != nil {
		return nil, fmt.Errorf("couldn't load filesystem repo: %w", err)
	}

	return repo, nil
}

func (r *Repo) AddDomainByPath(path string) error {
	r.domainSetMutex.Lock()
	defer r.domainSetMutex.Unlock()

	domain, err := newDomain(path)
	if err != nil {
		return fmt.Errorf("couldn't load domain at %s: %w", path, err)
	}

	r.domains = append(r.domains, domain)
	return nil
}

func (r *Repo) RemoveDomainByPath(path string) error {
	r.domainSetMutex.Lock()
	defer r.domainSetMutex.Unlock()

	for i, d := range r.domains {
		if d.filename == path {
			r.domains = append(r.domains[:i], r.domains[i+1:]...)
			return nil
		}
	}

	return fmt.Errorf("couldn't find domain at %s", path)
}

func (r *Repo) flagForRotate(ctx context.Context, d *domain, c cid.Cid) error {
	if d.currentCid.Equals(c) {
		if err := r.cueAdd(d.currentCid); err != nil {
			return fmt.Errorf("couldn't re-ensure CID was pinned for %s: %w", d.name, err)
		}
		return nil
	}

	if !d.currentCid.Defined() {
		if err := r.cueAdd(c); err != nil {
			return fmt.Errorf("couldn't pin CID for the first time for %s: %w", d.name, err)
		}
		if err := d.setCid(c); err != nil {
			return fmt.Errorf("couldn't store the new CID for %s (%s) in the DB: %w", d.name, c.String(), err)
		}
		log.Printf("Pinned %s for the first time: %s", d.name, c.String())
		return nil
	}

	if err := r.cueSwap(c, d.currentCid); err != nil {
		return fmt.Errorf("couldn't swap for the new CID (%s) for %s: %w", c.String(), d.name, err)
	}
	if err := d.setCid(c); err != nil {
		return fmt.Errorf("couldn't store the new CID for %s (%s) in the DB: %w", d.name, c.String(), err)
	}

	log.Printf("Swapped pin for %s, now %s", d.name, c.String())
	return nil
}

func (r *Repo) cueAdd(newCid cid.Cid) error {
	r.pinSetMutex.Lock()
	defer r.pinSetMutex.Unlock()

	if err := r.keepIfPinned(newCid); err != nil {
		return err
	}

	r.toPin = append(r.toPin, newCid)
	return nil
}

func (r *Repo) cueSwap(newCid, oldCid cid.Cid) error {
	r.pinSetMutex.Lock()
	defer r.pinSetMutex.Unlock()

	if err := r.keepIfPinned(newCid); err != nil {
		return err
	}

	r.toSwap = append(r.toSwap, cidPair{newCid, oldCid})
	return nil
}

func (r *Repo) cueRemove(oldCid cid.Cid) error {
	r.pinSetMutex.Lock()
	defer r.pinSetMutex.Unlock()

	r.toUnpin = append(r.toUnpin, oldCid)
	return nil
}

func (r *Repo) performPinChanges(ctx context.Context) {
	r.pinSetMutex.Lock()
	defer r.pinSetMutex.Unlock()

	doNotUnpin := make(map[string]struct{})
	for _, c := range r.toKeep {
		doNotUnpin[c.String()] = struct{}{}
	}

	var pair cidPair
	for len(r.toSwap) > 0 {
		pair, r.toSwap = r.toSwap[0], r.toSwap[1:]

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

	var c cid.Cid
	for len(r.toPin) > 0 {
		c, r.toPin = r.toPin[0], r.toPin[1:]

		doNotUnpin[c.String()] = struct{}{}
		if err := r.ipfs.Pin().Add(ctx, path.New(c.String()), options.Pin.Recursive(true)); err != nil {
			log.Printf("Unable to pin %s, this will be retried on the next check: %v", c.String(), err)
		}
	}

	for len(r.toUnpin) > 0 {
		c, r.toUnpin = r.toUnpin[0], r.toUnpin[1:]
		if _, ok := doNotUnpin[c.String()]; ok {
			continue
		}

		if err := r.ipfs.Pin().Rm(ctx, path.New(c.String())); err != nil {
			log.Printf("Unable to unpin %s, you should manually remove this: %v", c.String(), err)
		}
	}
}

// keepIfPinned ensures that if, by chance, a CID referenced by a domain is already pinned for some other reason,
// it will not be unpinned if that domain moves on to a different CID later.
// It expects that r.pinSetMutex.Lock() is already held by the executing thread
func (r *Repo) keepIfPinned(cid cid.Cid) error {
	// TODO: Add timeout
	ctx := context.Background()
	_, ok, err := r.ipfs.Pin().IsPinned(ctx, path.New(cid.String()))
	if err != nil {
		return err
	}

	if ok {
		r.toKeep = append(r.toKeep, cid)
		// TODO: Also write to file
	}

	return nil
}
