//go:build !linux

package main

import "io"

func openGPM(bool) (io.ReadWriteCloser, error) {
	return nil, nil
}

func readGPMEvents(io.Reader, chan<- gpmEvent) {}
