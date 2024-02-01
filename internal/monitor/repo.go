package monitor

import (
	"bufio"
	"context"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"sync"

	"github.com/ipfs/boxo/path"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/kubo/client/rpc"
	"github.com/ipfs/kubo/core/coreiface/options"
)

const keepFileName = ".cids-no-unpin"

type cidPair struct {
	new cid.Cid
	old cid.Cid
}

type Repo struct {
	rootDir string
	domains []*domain
	ipfs    *rpc.HttpApi

	pinSetMutex    sync.Mutex
	domainSetMutex sync.Mutex
	toKeep         map[cid.Cid]struct{}
	toPin          []cid.Cid
	toUnpin        []cid.Cid
	toSwap         []struct {
		new cid.Cid
		old cid.Cid
	}
}

func New(rootDir string, ipfs *rpc.HttpApi) (*Repo, error) {
	repo := &Repo{
		rootDir: rootDir,
		ipfs:    ipfs,
		toKeep:  make(map[cid.Cid]struct{}),
	}
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

	if err := repo.readKeepfile(); err != nil {
		return nil, fmt.Errorf("couldn't load keepfile: %w", err)
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
	log.Printf("➕ %s\n", domain.name)
	return nil
}

func (r *Repo) RemoveDomainByPath(path string) error {
	r.domainSetMutex.Lock()
	defer r.domainSetMutex.Unlock()

	for i, d := range r.domains {
		if d.filename == path {
			r.domains = append(r.domains[:i], r.domains[i+1:]...)
			log.Printf("➖ %s\n", d.name)
			if err := r.cueRemove(d.currentCid); err != nil {
				log.Printf("⚠️ Unable to cue unpin %s (for %s). Please unpin yourself: %v", d.currentCid, d.name, err)
			}
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
		if err := r.keepIfPinned(c); err != nil {
			return err
		}
		if err := r.cueAdd(c); err != nil {
			return fmt.Errorf("couldn't pin CID for the first time for %s: %w", d.name, err)
		}
		if err := d.setCid(c); err != nil {
			return fmt.Errorf("couldn't store the new CID for %s (%s) in the DB: %w", d.name, c.String(), err)
		}
		log.Printf("Pinned %s for the first time: %s", d.name, c.String())
		return nil
	}

	if err := r.keepIfPinned(c); err != nil {
		return err
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

	r.toPin = append(r.toPin, newCid)
	return nil
}

func (r *Repo) cueSwap(newCid, oldCid cid.Cid) error {
	r.pinSetMutex.Lock()
	defer r.pinSetMutex.Unlock()

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

	// These are not exclusively toKeep pins, but to ensure that multiple domains pointing to
	// the same CID are not unpinned if one of them is swapped.
	doNotUnpin := make(map[string]struct{})
	for c, _ := range r.toKeep {
		doNotUnpin[c.String()] = struct{}{}
	}

	var pair cidPair
	for len(r.toSwap) > 0 {
		pair, r.toSwap = r.toSwap[0], r.toSwap[1:]

		doNotUnpin[pair.new.String()] = struct{}{}

		if err := r.pin(ctx, pair.new); err != nil {
			doNotUnpin[pair.old.String()] = struct{}{}
			log.Printf("⚠️ Unable to swap in %s (to replace %s): %v", pair.new.String(), pair.old.String(), err)
			continue
		}
		log.Printf("📍 %s\n", pair.new.String())

		r.toUnpin = append(r.toUnpin, pair.old)
	}

	var c cid.Cid
	for len(r.toPin) > 0 {
		c, r.toPin = r.toPin[0], r.toPin[1:]

		doNotUnpin[c.String()] = struct{}{}
		if err := r.pin(ctx, c); err != nil {
			log.Printf("⚠️ Unable to pin %s, this will be retried on the next check: %v", c.String(), err)
		}

		log.Printf("📍 %s\n", c.String())
	}

	for len(r.toUnpin) > 0 {
		c, r.toUnpin = r.toUnpin[0], r.toUnpin[1:]
		if _, ok := doNotUnpin[c.String()]; ok {
			fmt.Printf("🛟 Not removing %s as it was manually pinned\n", c.String())
			continue
		}

		if err := r.unpin(ctx, c); err != nil {
			log.Printf("⚠️ Unable to unpin %s, you should manually unpin it: %v", c.String(), err)
		}
		log.Printf("🚮 %s\n", c.String())
	}
}

func (r *Repo) pin(ctx context.Context, c cid.Cid) error {
	cp, err := path.NewPathFromSegments("ipfs", c.String())
	if err != nil {
		return err
	}
	return r.ipfs.Pin().Add(ctx, cp, options.Pin.Recursive(true))
}

func (r *Repo) unpin(ctx context.Context, c cid.Cid) error {
	cp, err := path.NewPathFromSegments("ipfs", c.String())
	if err != nil {
		return err
	}
	return r.ipfs.Pin().Rm(ctx, cp)
}

// keepIfPinned ensures that if, by chance, a CID referenced by a domain is already pinned for some other reason,
// it will not be unpinned if that domain moves on to a different CID later.
func (r *Repo) keepIfPinned(c cid.Cid) error {
	r.pinSetMutex.Lock()
	defer r.pinSetMutex.Unlock()

	// TODO: Add timeout
	ctx := context.Background()
	cp, err := path.NewPathFromSegments("ipfs", c.String())
	if err != nil {
		return err
	}
	_, ok, err := r.ipfs.Pin().IsPinned(ctx, cp)
	if err != nil || !ok {
		return err
	}

	r.toKeep[c] = struct{}{}
	if err := r.writeKeepfile(); err != nil {
		return fmt.Errorf("couldn't write keepfile to ensure %s is not unpinned: %w", c.String(), err)
	}

	return nil
}

// writeKeepfile writes the CIDs in r.toKeep to the keepfile
// It expects that r.pinSetMutex.Lock() is already held by the executing thread
func (r *Repo) writeKeepfile() error {
	f, err := os.OpenFile(filepath.Join(r.rootDir, keepFileName), os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	for c, _ := range r.toKeep {
		if _, err := f.WriteString(c.String() + "\n"); err != nil {
			return err
		}
	}
	return nil
}

// readKeepfile loads the CIDs from the keepfile into r.toKeep
// It expects to have exclusive access to the keepfile. Claim r.pinSetMutex.Lock() if needed.
func (r *Repo) readKeepfile() error {
	file, err := os.Open(filepath.Join(r.rootDir, keepFileName))
	if err != nil {
		// There doesn't need to be a valid keepfile
		return nil
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		c, err := cid.Decode(line)
		if err != nil {
			return fmt.Errorf("couldn't decode CID (%s) in keepfile: %w", line, err)
		}
		r.toKeep[c] = struct{}{}
	}

	return nil
}
