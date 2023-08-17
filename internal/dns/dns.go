package dns

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/ipfs/go-cid"
	"github.com/libp2p/go-libp2p/core/peer"

	dnslink "github.com/dnslink-std/go"
)

var ErrNotDNSLinkDomain = errors.New("no usable dnslink record found")
var ErrTooManyDomainRedirects = errors.New("too many domain redirects")
var ErrDomainRedirectLoop = errors.New("domain redirect loop detected and rejected")

const maxDNSLinkRedirects = 4
const minRetryTime = 5 * time.Minute

type contextKey string

var previousDomainsKey = contextKey("previousDomains")
var maxTTLKey = contextKey("maxTTL")

func LookupDNSLinkCID(ctx context.Context, domain string) (cid.Cid, time.Duration, error) {
	// TODO: Context for timeout?
	res, err := dnslink.Resolve(domain)
	if err != nil {
		return cid.Cid{}, time.Duration(-1), err
	}

	// Can I use path "github.com/ipfs/boxo/coreiface/path" here?

	if len(res.Links["ipfs"]) > 0 {
		link := res.Links["ipfs"][0]

		c, err := cid.Decode(link.Identifier)
		if err != nil {
			return cid.Cid{}, time.Duration(-2), err
		}

		return c, minDurationFromContext(ctx, link.Ttl), nil
	}

	if len(res.Links["ipns"]) == 0 {
		return cid.Cid{}, time.Duration(-3), ErrNotDNSLinkDomain
	}

	link := res.Links["ipns"][0]

	// TODO: resolve the CID, because https://github.com/ipfs/kubo/issues/1467
	if _, err := peer.Decode(link.Identifier); err == nil {
		return cid.Cid{}, time.Duration(-4), fmt.Errorf("ipns peer IDs not supported yet")
	}

	var previousDomains []string
	var ok bool
	pd := ctx.Value(previousDomainsKey)
	if previousDomains, ok = pd.([]string); !ok {
		return cid.Cid{}, time.Duration(-5), fmt.Errorf("previousDomains in context was not a slice of string, was %#v (%T)", pd, pd)
	}

	if len(previousDomains) >= maxDNSLinkRedirects {
		return cid.Cid{}, time.Duration(-6), ErrTooManyDomainRedirects
	}

	for _, prevDomain := range previousDomains {
		if link.Identifier == prevDomain {
			return cid.Cid{}, time.Duration(-7), ErrDomainRedirectLoop
		}
	}

	ctx = context.WithValue(ctx, previousDomainsKey, append(previousDomains, domain))
	ctx = context.WithValue(ctx, maxTTLKey, minDurationFromContext(ctx, link.Ttl))

	return LookupDNSLinkCID(ctx, link.Identifier)
}

func minDurationFromContext(ctx context.Context, seconds uint32) time.Duration {
	dnsTTL := time.Duration(seconds) * time.Second
	if prevTTL, ok := ctx.Value(maxTTLKey).(time.Duration); ok {
		if dnsTTL > prevTTL {
			dnsTTL = prevTTL
		}
	}

	if dnsTTL < minRetryTime {
		return minRetryTime
	}

	return dnsTTL
}
