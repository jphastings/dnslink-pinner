package monitor

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/jphastings/dnslink-pinner/internal/dns"
)

const rotateTimeout = 15 * time.Second
const minRecheckInterval = 24 * time.Hour

func (r *Repo) Monitor() error {
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

		time.Sleep(minRecheckInterval)
	}
}

func (r *Repo) checkAndRotate(d *domain) (time.Duration, error) {
	ctx, cancel := context.WithTimeout(context.Background(), rotateTimeout)
	defer cancel()

	c, dnsTTL, err := dns.LookupDNSLinkCID(ctx, d.name)
	if err != nil {
		return time.Duration(0), err
	}
	log.Printf("Found: %s => %s\n", d.name, c.String())

	if err := d.setCid(c); err != nil {
		return time.Duration(0), err
	}

	r.flagForRotate(ctx, d, c)
	return dnsTTL, nil
}
