package step

import (
	"crypto/sha256"
	"hash"
	"io"
)

type ProgressTrackReader struct {
	tracker Tracker
	io.ReadCloser
	sha hash.Hash
}

func NewProgressTrackStream(tracker Tracker, reader io.ReadCloser) *ProgressTrackReader {
	return &ProgressTrackReader{
		tracker:    tracker,
		ReadCloser: reader,
		sha:        sha256.New(),
	}
}

func (p *ProgressTrackReader) Read(b []byte) (n int, err error) {
	n, err = p.ReadCloser.Read(b)
	p.tracker.Increment(n)
	p.sha.Write(b[0:n])
	return n, err
}

func (p *ProgressTrackReader) SumSha() []byte {
	return p.sha.Sum(nil)
}
