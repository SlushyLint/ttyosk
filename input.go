package main

import (
	"bytes"
)

type inputDecoder struct {
	pending []byte
}

func (d *inputDecoder) feed(input []byte) [][]byte {
	d.pending = append(d.pending, input...)
	var events [][]byte
	for len(d.pending) > 0 {
		if d.pending[0] == 27 {
			if bytes.HasPrefix(d.pending, []byte("\033[")) {
				if len(d.pending) == 2 {
					break
				}
				if bytes.HasPrefix(d.pending, []byte("\033[<")) {
					end := bytes.IndexAny(d.pending[3:], "Mm")
					if end < 0 {
						break
					}
					end += 4
					events = append(events, append([]byte(nil), d.pending[:end]...))
					d.pending = d.pending[end:]
					continue
				}
				if d.pending[2] >= 'A' && d.pending[2] <= 'D' {
					events = append(events, append([]byte(nil), d.pending[:3]...))
					d.pending = d.pending[3:]
					continue
				}
			}
		}
		events = append(events, append([]byte(nil), d.pending[:1]...))
		d.pending = d.pending[1:]
	}
	return events
}

func (d *inputDecoder) flush() [][]byte {
	var events [][]byte
	for _, value := range d.pending {
		events = append(events, []byte{value})
	}
	d.pending = nil
	return events
}
