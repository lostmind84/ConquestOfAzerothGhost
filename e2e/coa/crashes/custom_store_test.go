//go:build e2e

package crashes_test

import (
	"bytes"
	"encoding/binary"
	"testing"
	"time"

	"github.com/azerothcore/AzerothGhost/e2e/e2eharness"
)

func TestCustomStorePurchaseAfterQueuedDispatch(t *testing.T) {
	const (
		currency = uint32(2499003)
		bundle   = uint32(2615021)
	)
	bot := e2eharness.NewSolo(t, e2eharness.ScenarioOpts{
		Prefix: "StoreV",
		Race:   e2eharness.RaceOrc,
		Class:  13,
	})
	bot.AddItemWait(t, currency, 1)

	queryResults := make(chan []byte, 1)
	cancel := bot.World.AddPacketHook(func(opcode uint16, data []byte) {
		if opcode == 0x06BA {
			select {
			case queryResults <- append([]byte(nil), data...):
			default:
			}
		}
	})
	defer cancel()
	query := binary.LittleEndian.AppendUint32(nil, 10)
	if err := bot.World.SendRawPacket(0x06B9, query); err != nil {
		t.Fatal(err)
	}
	select {
	case result := <-queryResults:
		end := bytes.IndexByte(result, 0)
		if end < 0 || string(result[:end]) != "QUERY_CUSTOM_STORE_OK" || len(result) < end+5 {
			t.Fatalf("unexpected custom store query result: %q", result)
		}
		if count := binary.LittleEndian.Uint32(result[end+1:]); count == 0 {
			t.Fatal("class bundle store returned no records")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("custom store query did not answer")
	}

	purchase := binary.LittleEndian.AppendUint32(nil, bundle)
	purchase = binary.LittleEndian.AppendUint32(purchase, 1)
	if err := bot.World.SendRawPacket(0x06BB, purchase); err != nil {
		t.Fatal(err)
	}
	if _, err := bot.Session.WaitItemPushEntry(bundle, 5*time.Second); err != nil {
		t.Fatalf("custom store did not deliver bundle %d: %v", bundle, err)
	}
	e2eharness.ProbeWorldAlive(t, bot, 0)
}
