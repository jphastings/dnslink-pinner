package monitor

import (
	"context"
	"fmt"
	"io/fs"
	"path"
	"time"

	"github.com/jphastings/dnslink-pinner/internal/dns"
)

type Repo struct {
	domains map[string]domain
	fs      fs.FS
}

type domain struct {
	path      string
	nextCheck time.Time
}

func (d domain) name() string { return path.Base(d.path) }

func New(repo fs.FS) (*Repo, error) {

	return &Repo{
		domains: map[string]domain{"www.byjp.me": {path: "/www.byjp.me"}},
		fs:      repo,
	}, nil
}

func (r *Repo) Monitor() error {
	// The fallback for the latest second check
	delayUntilNextPause := time.Hour * 24

	for _, d := range r.domains {
		nextCheck := time.Until(d.nextCheck)
		if nextCheck > 0 {
			if delayUntilNextPause > nextCheck {
				delayUntilNextPause = nextCheck
			}
			continue
		}

		cid, _, err := dns.LookupDNSLinkCID(context.Background(), d.name())
		// TODO: This isn't right; it should be retried
		if err != nil {
			return err
		}

		_ = cid
	}

	return fmt.Errorf("not implemented")
}
