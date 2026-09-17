//go:build e2e

package crashes_test

import (
	"sync"
	"testing"
	"time"

	"github.com/azerothcore/AzerothGhost/e2e/e2eharness"
)

// Main project issue #401: using Feather of Ancients: Azeroth (134989) crashes the server.
// Reported by a level 11 Reaper at 9316.13, -7210.24, 15.10 in Eversong Woods (map 530).
func TestCrash_FeatherOfAncients(t *testing.T) {
	const (
		itemFeatherOfAncients uint32 = 134989
		spellDummy            uint32 = 18282  // item spell 1
		spellUnlockFlightPath uint32 = 979610 // item spell 2
		classReaper           uint8  = 30
		raceBloodElf          uint8  = 10
		mapOutland            uint32 = 530
	)
	bot := e2eharness.NewSolo(t, e2eharness.ScenarioOpts{Prefix: "Feath", Race: raceBloodElf, Class: classReaper, Level: 11})
	bot.Teleport(t, 9316.13, -7210.24, 15.1011, mapOutland)
	bot.AddItem(t, itemFeatherOfAncients, 1)

	var mu sync.Mutex
	results := map[uint32]string{}
	cancel := bot.World.AddSpellCastResultHook(func(spellID uint32, success bool, failReason uint8) {
		if spellID != spellDummy && spellID != spellUnlockFlightPath {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		if success {
			results[spellID] = "success"
		} else {
			results[spellID] = "failed: " + e2eharness.SpellFailReasonName(failReason)
		}
	})
	defer cancel()

	bot.UseItemEntry(t, itemFeatherOfAncients, 0)
	time.Sleep(settleDelay + 5*time.Second) // Unlock Flight Paths has a cast time
	e2eharness.ProbeWorldAlive(t, bot, 401)
	mu.Lock()
	defer mu.Unlock()
	if len(results) == 0 {
		t.Errorf("precondition: no cast result for the feather's spells; the item was not used")
	}
	t.Logf("cast results: %v", results)
}
