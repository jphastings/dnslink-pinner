package monitor

import (
	"fmt"
	"net/url"
	"os"
	"path"

	"github.com/ipfs/go-cid"
)

type domain struct {
	filename   string
	name       string
	currentCid cid.Cid
	errorCount uint
}

func isFQDN(s string) bool {
	u, err := url.Parse("http://" + s)
	if err != nil {
		return false
	}

	return u.Hostname() == s
}

func newDomain(filename string) (*domain, error) {
	cidStr, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	name := path.Base(filename)
	if !isFQDN(name) {
		return nil, fmt.Errorf("the file %s (in %s) is not a fully qualified domain name", name, path.Dir(filename))
	}

	if len(cidStr) == 0 {
		return &domain{
			name:     name,
			filename: filename,
		}, nil
	}

	currentCid, err := cid.Decode(string(cidStr))
	if err != nil {
		return nil, err
	}

	return &domain{
		name:       path.Base(filename),
		currentCid: currentCid,
		filename:   filename,
	}, nil
}

func (d *domain) setCid(c cid.Cid) error {
	d.currentCid = c
	if err := os.WriteFile(d.filename, []byte(c.String()), 0644); err != nil {
		return fmt.Errorf("couldn't write CID to file: %w", err)
	}

	return nil
}
