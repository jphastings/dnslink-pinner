package dns

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/ipfs/go-cid"
	"github.com/libp2p/go-libp2p/core/peer"
)

var ErrNotDNSLinkDomain = errors.New("no dnslink record found")
var dnslinkPrefix = "dnslink="
var maxDNSLinkRedirects = 4

type contextKey string

var previousDomainsKey = contextKey("previousDomains")

func LookupDNSLinkCID(ctx context.Context, domain string) (cid.Cid, time.Duration, error) {
	// TODO: How to get TTL out??

	// TODO: Use resolver to allow for context
	txts, err := net.LookupTXT(fmt.Sprintf("_dnslink.%s", domain))
	if err != nil {
		return cid.Cid{}, time.Duration(0), err
	}

	ipfsStr := ""
	for _, txt := range txts {
		fmt.Println(txt)
		if strings.HasPrefix(txt, dnslinkPrefix) {
			ipfsStr = strings.TrimPrefix(txt, dnslinkPrefix)
			break
		}
	}
	if ipfsStr == "" {
		return cid.Cid{}, time.Duration(0), ErrNotDNSLinkDomain
	}

	if strings.HasPrefix(ipfsStr, "/ipfs/") {
		c, err := cid.Decode(ipfsStr[6:])
		// TODO: time.Hour is a fallback; use TTL?
		return c, time.Hour, err

	}

	if !strings.HasPrefix(ipfsStr, "/ipns/") {
		return cid.Cid{}, time.Duration(0), fmt.Errorf("invalid dnslink record: %s", ipfsStr)
	}

	c, err := cid.Decode(ipfsStr[6:])
	if err != nil {
		_, err := peer.FromCid(c)
		if err != nil {
			return cid.Cid{}, time.Duration(0), err
		}
		// TODO: time.Hour is a fallback; use TTL?
		return c, time.Hour, nil

	}

	if _, err := net.LookupHost(ipfsStr[6:]); err != nil {
		return cid.Cid{}, time.Duration(0), err
	}

	var previousDomains []string
	var ok bool
	pd := ctx.Value(previousDomainsKey)
	if previousDomains, ok = pd.([]string); !ok {
		return cid.Cid{}, time.Duration(0), fmt.Errorf("previousDomains in context was not a slice of string, was %#v (%T)", pd, pd)
	}

	if len(previousDomains) >= maxDNSLinkRedirects {
		return cid.Cid{}, time.Duration(0), fmt.Errorf("too many domain redirects")
	}

	redirectedDomain := ipfsStr[6:]
	for _, prevDomain := range previousDomains {
		if redirectedDomain == prevDomain {
			return cid.Cid{}, time.Duration(0), fmt.Errorf("domain redirect loop detected and rejected")
		}
	}

	return LookupDNSLinkCID(
		context.WithValue(ctx, previousDomainsKey, append(previousDomains, domain)),
		redirectedDomain,
	)
}
