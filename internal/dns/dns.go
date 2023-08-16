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

	"github.com/projectdiscovery/retryabledns"
)

var ErrNotDNSLinkDomain = errors.New("no dnslink record found")

const dnslinkPrefix = "dnslink="
const maxDNSLinkRedirects = 4
const maxDNSRetries = 5

type contextKey string

var previousDomainsKey = contextKey("previousDomains")
var maxTTLKey = contextKey("maxTTL")

func LookupDNSLinkCID(ctx context.Context, domain string) (cid.Cid, time.Duration, error) {
	// TODO: How to get TTL out??
	dns, err := retryabledns.New([]string{"1.1.1.1", "8.8.8.8"}, maxDNSRetries)
	if err != nil {
		return cid.Cid{}, time.Duration(0), err
	}

	// TODO: Context for timeout?
	data, err := dns.TXT(fmt.Sprintf("_dnslink.%s", domain))
	if err != nil {
		return cid.Cid{}, time.Duration(0), err
	}

	dnsTTL := time.Duration(data.TTL) * time.Second
	if prevTTL, ok := ctx.Value(maxTTLKey).(time.Duration); ok {
		if dnsTTL > prevTTL {
			dnsTTL = prevTTL
		}
	}

	ipfsStr := ""
	for _, txt := range data.TXT {
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
		return c, dnsTTL, err

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
		return c, dnsTTL, nil
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

	ctx = context.WithValue(ctx, previousDomainsKey, append(previousDomains, domain))
	ctx = context.WithValue(ctx, maxTTLKey, dnsTTL)

	return LookupDNSLinkCID(ctx, redirectedDomain)
}
