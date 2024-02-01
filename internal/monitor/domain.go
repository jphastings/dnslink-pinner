package monitor

import (
	"fmt"
	"os"
	"path"
	"time"

	"github.com/ipfs/go-cid"
)

type domain struct {
	filename   string
	name       string
	currentCid cid.Cid
	nextCheck  time.Time
	errorCount uint
}

func newDomain(filename string) (*domain, error) {
	cidStr, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	if len(cidStr) == 0 {
		return &domain{
			name:     path.Base(filename),
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
