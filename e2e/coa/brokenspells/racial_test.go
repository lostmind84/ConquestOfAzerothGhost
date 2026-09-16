//go:build e2e

package brokenspells_test

import (
	"testing"

	"github.com/azerothcore/AzerothGhost/e2e/e2eharness"
)

// Gift of the Naaru variants granted to CoA Draenei (spell power, attack power, hybrid).
var giftOfTheNaaru = []uint32{814280, 814281, 814282}

// Main project issue #1409: a level 3 Draenei Sun Cleric has no Gift of the Naaru.
// The live class baseline SQL only lists 814282 under class 17 rows, so a Draenei Knight of Xoroth is the control.
//
//	go test -tags=e2e ./e2e/coa/brokenspells -run GiftOfTheNaaru -count=1 -v
func TestDraenei_GiftOfTheNaaruKnown(t *testing.T) {
	for _, c := range []struct {
		name   string
		prefix string
		class  uint8
	}{{"KnightOfXoroth", "DrKox", classKnightXoroth}, {"SunCleric", "DrSun", classSunCleric}} {
		t.Run(c.name, func(t *testing.T) {
			bot := newBot(t, c.prefix, e2eharness.RaceDraenei, c.class, 3)
			bot.Relog(t) // the spellbook is only complete in SMSG_INITIAL_SPELLS

			for _, id := range giftOfTheNaaru {
				if bot.World.KnowsSpell(id) || bot.SpellbookCount(id) > 0 {
					t.Logf("E2E_PASS: Draenei %s knows Gift of the Naaru %d", c.name, id)
					return
				}
			}
			t.Errorf("E2E_FAIL: Draenei %s knows none of Gift of the Naaru %v (#1409)", c.name, giftOfTheNaaru)
		})
	}
}
