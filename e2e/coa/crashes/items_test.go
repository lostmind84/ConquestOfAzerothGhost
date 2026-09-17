//go:build e2e

package crashes_test

import (
	"bytes"
	"encoding/binary"
	"sync/atomic"
	"testing"
	"time"

	"github.com/azerothcore/AzerothGhost/client"
	"github.com/azerothcore/AzerothGhost/e2e/e2eharness"
)

// Main project issue #401: using Feather of Ancients: Azeroth (134989) crashes the server.
// Reported by a level 11 Reaper at 9316.13, -7210.24, 15.10 in Eversong Woods (map 530).
//
// The item has two on-use spells with one charge. Dummy Spell (18282) uses the last charge and
// destroys the item; an item that was never saved is deleted at once, and Unlock Flight Paths
// (979610) is then prepared with the deleted item. The crash depends on the freed memory, so the
// feather is added and used several times, each time before any save.
func TestCrash_FeatherOfAncients(t *testing.T) {
	const (
		itemFeatherOfAncients uint32 = 134989
		spellDummy            uint32 = 18282 // item spell 1, destroys the item
		classReaper           uint8  = 30
		raceBloodElf          uint8  = 10
		mapOutland            uint32 = 530
		uses                         = 5
	)
	bot := e2eharness.NewSolo(t, e2eharness.ScenarioOpts{Prefix: "Feath", Race: raceBloodElf, Class: classReaper, Level: 11})
	bot.Teleport(t, 9316.13, -7210.24, 15.1011, mapOutland)

	var dummyCasts atomic.Int32
	self := bot.World.CharGUID()
	defer bot.World.AddPacketHook(func(opcode uint16, data []byte) {
		if opcode != client.SmsgSpellGo {
			return
		}
		r := bytes.NewReader(data)
		readPackedGUID(r) // cast item or caster
		if readPackedGUID(r) != self {
			return
		}
		var castCount uint8
		var id uint32
		if binary.Read(r, binary.LittleEndian, &castCount) == nil &&
			binary.Read(r, binary.LittleEndian, &id) == nil && id == spellDummy {
			dummyCasts.Add(1)
		}
	})()

	for use := 1; use <= uses; use++ {
		_ = bot.World.SetTarget(self)
		bot.GM(t, ".cooldown")
		bot.AddAndUseItem(t, itemFeatherOfAncients, 0)
		time.Sleep(settleDelay)
		assertWorldAnswers(t, bot, 401)
	}
	if dummyCasts.Load() == 0 {
		t.Errorf("precondition: Dummy Spell (%d) never went off; the feather was not used", spellDummy)
	}
}

// assertWorldAnswers checks that the worldserver still answers a GM command. A crash can close the
// session after the command was sent, so the answer itself is the proof.
func assertWorldAnswers(t *testing.T, bot *e2eharness.ScenarioBot, issue int) {
	t.Helper()
	got := make(chan struct{}, 1)
	cancel := bot.World.AddPacketHook(func(opcode uint16, _ []byte) {
		if opcode == client.SmsgMessageChat {
			select {
			case got <- struct{}{}:
			default:
			}
		}
	})
	defer cancel()
	if err := bot.World.SendGMCommand(".gps"); err != nil {
		e2eharness.ConfirmedBugf(t, issue, "GM command failed, worldserver likely crashed: %v", err)
	}
	select {
	case <-got:
	case <-time.After(10 * time.Second):
		e2eharness.ConfirmedBugf(t, issue, "no answer from the worldserver within 10 s, it likely crashed")
	}
}
