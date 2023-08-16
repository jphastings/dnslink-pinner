package monitor

import (
	"context"
	"fmt"
	"path"
	"time"

	"github.com/ipfs/go-cid"
)

type domain struct {
	path       string
	currentCid cid.Cid
	nextCheck  time.Time
	errorCount uint
}

func (d domain) name() string { return path.Base(d.path) }

func (d domain) rotate(ctx context.Context, c cid.Cid) error {
	if d.currentCid.Equals(c) {
		return nil
	}
	previousCid := d.currentCid

	// TODO: Pin new CID
	if err := d.setCurrentCid(c); err != nil {
		// TODO: Try to unpin, so we don't have a pin without a reference
		return fmt.Errorf("not implemented")
	}

	if previousCid.Defined() {
		// TODO: Unpin old CID
		return fmt.Errorf("not implemented")
	}

	return nil
}

func (d domain) setCurrentCid(c cid.Cid) error {
	return fmt.Errorf("not implemented")
}
