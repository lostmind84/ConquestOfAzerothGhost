//go:build e2e

package crashes_test

import (
	"bytes"
	"encoding/binary"
	"testing"
	"time"

	"github.com/azerothcore/AzerothGhost/e2e/e2eharness"
)

func TestCharacterSelectionCallbacksFromSocket(t *testing.T) {
	bot := e2eharness.NewSolo(t, e2eharness.ScenarioOpts{
		Prefix: "CharQ",
		Race:   e2eharness.RaceOrc,
		Class:  13,
	})
	type response struct {
		opcode  uint16
		payload []byte
	}
	responses := make(chan response, 8)
	cancel := bot.World.AddPacketHook(func(opcode uint16, data []byte) {
		if opcode == 0x075E || opcode == 0x075F || opcode == 0x0760 {
			select {
			case responses <- response{opcode, append([]byte(nil), data...)}:
			default:
			}
		}
	})
	defer cancel()

	if err := bot.World.SendRawPacket(0x0037, nil); err != nil {
		t.Fatal(err)
	}
	select {
	case answer := <-responses:
		if answer.opcode != 0x075E {
			t.Fatalf("character enumeration answered with opcode 0x%04X", answer.opcode)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("character enumeration callback did not answer")
	}

	missingGuid := binary.LittleEndian.AppendUint32(nil, ^uint32(0))
	for _, request := range []struct {
		opcode uint16
		answer uint16
	}{
		{0x072E, 0x075F},
		{0x072F, 0x0760},
	} {
		if err := bot.World.SendRawPacket(request.opcode, missingGuid); err != nil {
			t.Fatal(err)
		}
		select {
		case answer := <-responses:
			if answer.opcode != request.answer || !bytes.Contains(answer.payload, []byte("NOT_FOUND")) {
				t.Fatalf("unexpected callback result for 0x%04X: 0x%04X %q",
					request.opcode, answer.opcode, answer.payload)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("callback for 0x%04X did not answer", request.opcode)
		}
	}
}
