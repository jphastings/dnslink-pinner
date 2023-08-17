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
	delays := make([]time.Duration, len(r.domains))

	for {
		var wg sync.WaitGroup

		for i, d := range r.domains {
			nextCheck := time.Until(d.nextCheck)
			if nextCheck > 0 {
				delays[i] = nextCheck
				continue
			}

			wg.Add(1)
			go func(i int, d *domain) {
				defer wg.Done()

				dnsTTL, err := r.checkAndRotate(d)
				if err != nil {
					d.errorCount += 1
					log.Printf("Unable to check and rotate CID for %s: %v", d.name, err)
					return
				}
				delays[i] = dnsTTL
			}(i, d)
		}

		wg.Wait()

		r.performPinChanges(context.Background())

		time.Sleep(minDuration(minRecheckInterval, delays...))
	}
}

func (r *Repo) checkAndRotate(d *domain) (time.Duration, error) {
	ctx, cancel := context.WithTimeout(context.Background(), rotateTimeout)
	defer cancel()

	c, dnsTTL, err := dns.LookupDNSLinkCID(ctx, d.name)
	if err != nil {
		return time.Duration(0), err
	}

	r.flagForRotate(ctx, d, c)
	return dnsTTL, nil
}
