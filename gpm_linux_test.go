//go:build linux

package main

import (
	"encoding/binary"
	"testing"
)

func TestDecodeGPMEvent(t *testing.T) {
	data := make([]byte, gpmEventSize)
	data[0] = gpmLeftButton
	binary.NativeEndian.PutUint16(data[8:10], 23)
	binary.NativeEndian.PutUint16(data[10:12], 18)
	binary.NativeEndian.PutUint32(data[12:16], gpmDown)

	event, ok := decodeGPMEvent(data)
	if !ok {
		t.Fatal("expected valid GPM event")
	}
	if event.x != 23 || event.y != 18 || !event.pressed {
		t.Fatalf("expected left-button press at 23,18; got %+v", event)
	}

	binary.NativeEndian.PutUint32(data[12:16], gpmMove)
	event, _ = decodeGPMEvent(data)
	if event.pressed {
		t.Fatal("expected a GPM move event not to activate a key")
	}

	data[0] = 1
	data[1] = 1
	binary.NativeEndian.PutUint32(data[12:16], gpmDown|gpmDrag)
	event, _ = decodeGPMEvent(data)
	if !event.rightPressed || !event.shift || !event.drag {
		t.Fatalf("expected GPM right-button drag with Shift, got %+v", event)
	}
}

func TestDecodeGPMEventRejectsInvalidSize(t *testing.T) {
	if _, ok := decodeGPMEvent(make([]byte, gpmEventSize-1)); ok {
		t.Fatal("expected invalid GPM event size to be rejected")
	}
}
