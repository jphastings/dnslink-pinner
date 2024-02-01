package monitor

import (
	"context"
	"log"
	"os"
	"sync"
	"time"

	"github.com/jphastings/dnslink-pinner/internal/dns"

	"github.com/fsnotify/fsnotify"
)

const rotateTimeout = 15 * time.Second
const minRecheckInterval = 15 * time.Minute

func (r *Repo) Monitor() error {
	requestRefresh := make(chan struct{}, 1)
	cl, err := watchDir(r, func() { requestRefresh <- struct{}{} })
	if err != nil {
		return err
	}
	defer cl.Close()

	for {
		var wg sync.WaitGroup

		r.domainSetMutex.Lock()
		domains := make([]*domain, len(r.domains))
		copy(domains, r.domains)
		r.domainSetMutex.Unlock()

		for _, d := range domains {
			wg.Add(1)
			go func(d *domain) {
				defer wg.Done()

				_, err := r.checkAndRotate(d)
				if err != nil {
					d.errorCount += 1
					log.Printf("Unable to check and rotate CID for %s: %v", d.name, err)
					return
				}
			}(d)
		}

		wg.Wait()

		r.performPinChanges(context.Background())

		select {
		case <-requestRefresh:
		case <-time.After(minRecheckInterval):
		}
	}
}

type Closer interface {
	Close() error
}

func watchDir(r *Repo, requestRefresh func()) (Closer, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	go func() {
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				if event.Has(fsnotify.Create) {
					if err := r.AddDomainByPath(event.Name); err != nil {
						log.Printf("😩 Unable to add new domain to checker %s: %v\n", event.Name, err)
					}
					requestRefresh()
				}
				if event.Has(fsnotify.Remove) {
					if err := r.RemoveDomainByPath(event.Name); err != nil {
						log.Printf("😩 Unable to remove domain to checker %s: %v\n", event.Name, err)
					}
					requestRefresh()
				}
				// On some operating systems a delete is modelled as a rename into the trash
				if event.Has(fsnotify.Rename) {
					if _, err := os.Stat(event.Name); err != nil && os.IsNotExist(err) {
						if err := r.RemoveDomainByPath(event.Name); err != nil {
							log.Printf("😩 Unable to remove domain to checker %s: %v\n", event.Name, err)
						}
					}
					requestRefresh()
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				log.Printf("📁 issue while checking filesystem: %v\n", err)
			}
		}
	}()

	if err := watcher.Add(r.rootDir); err != nil {
		return watcher, err
	}

	return watcher, nil
}

func (r *Repo) checkAndRotate(d *domain) (time.Duration, error) {
	ctx, cancel := context.WithTimeout(context.Background(), rotateTimeout)
	defer cancel()

	c, dnsTTL, err := dns.LookupDNSLinkCID(ctx, d.name)
	if err != nil {
		return time.Duration(0), err
	}
	log.Printf("👀 %s (%s)\n", c.String(), d.name)

	if err := d.setCid(c); err != nil {
		return time.Duration(0), err
	}

	r.flagForRotate(ctx, d, c)
	return dnsTTL, nil
}
