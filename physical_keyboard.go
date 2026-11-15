package main

type physicalKeyEvent struct {
	code  uint16
	value int32
	err   error
}

const (
	physicalKeyEsc   uint16 = 1
	physicalKeyEnter uint16 = 28
	physicalKeyH     uint16 = 35
	physicalKeyJ     uint16 = 36
	physicalKeyK     uint16 = 37
	physicalKeyL     uint16 = 38
	physicalKeySpace uint16 = 57
	physicalKeyUp    uint16 = 103
	physicalKeyLeft  uint16 = 105
	physicalKeyRight uint16 = 106
	physicalKeyDown  uint16 = 108
)

func (s *keyboardState) handlePhysicalKey(event physicalKeyEvent) bool {
	if event.err != nil {
		s.lastError = event.err
		return false
	}
	if event.value != 1 && event.value != 2 {
		return false
	}

	switch event.code {
	case physicalKeyH, physicalKeyLeft:
		s.move(-1, 0)
	case physicalKeyJ, physicalKeyDown:
		s.move(0, 1)
	case physicalKeyK, physicalKeyUp:
		s.move(0, -1)
	case physicalKeyL, physicalKeyRight:
		s.move(1, 0)
	case physicalKeySpace, physicalKeyEnter:
		s.insertSelectedKey()
	case physicalKeyEsc:
		return true
	}
	return false
}
