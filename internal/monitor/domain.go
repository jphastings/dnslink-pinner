package monitor

import (
	"fmt"
	"io"
	"io/fs"
	"path"
	"time"

	"github.com/ipfs/go-cid"
)

type domain struct {
	repoFS     fs.FS
	filename   string
	name       string
	currentCid cid.Cid
	nextCheck  time.Time
	errorCount uint
}

func newDomain(repoFS fs.FS, filename string) (*domain, error) {
	f, err := repoFS.Open(filename)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	cidStr, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}

	currentCid, err := cid.Parse(cidStr)
	if err != nil {
		return nil, err
	}

	return &domain{
		name:       path.Base(filename),
		currentCid: currentCid,
	}, nil
}

func (d *domain) setCid(c cid.Cid) error {
	d.currentCid = c

	return fmt.Errorf("not implemented")
}
