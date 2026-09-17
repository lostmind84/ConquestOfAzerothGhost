//go:build e2e

package systems_test

import (
	"testing"
	"time"

	"github.com/azerothcore/AzerothGhost/e2e/e2eharness"
)

// Main project issue #322: casting the Necromancer's Graveyard crashes the worldserver.
// The ranks are the base spell (level 60 class ability) and its two talent replacements.
//
//	go test -tags=e2e ./e2e/coa/systems -run Graveyard -count=1 -v
func TestCrash_Graveyard(t *testing.T) {
	for _, tc := range []struct {
		name  string
		spell uint32
		level int
	}{
		{"Base60", 805197, 60},
		{"Base1", 805197, 1}, // the report was filed at level 1
		{"Deader", 805412, 60},
		{"Deader2", 805465, 60},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bot := newBot(t, "Grave", e2eharness.RaceUndead, classNecromancer, tc.level)
			bot.Learn(t, tc.spell)
			if !waitKnows(t, bot, tc.spell, 3*time.Second) {
				t.Fatalf("precondition: Graveyard %d not learned", tc.spell)
			}
			bot.CheatPower(t)
			enemies := []uint64{
				spawnEnemy(t, bot, creatureHarvestGolem, tc.level),
				spawnEnemy(t, bot, creatureHarvestGolem, tc.level),
			}
			bot.CombatReady(t)
			x, y, z, _ := bot.Pos()
			var res e2eharness.SpellCastResult
			for attempt := 0; attempt < 3; attempt++ {
				res = bot.CastAtPosition(t, tc.spell, x+4, y, z, castTimeout)
				if res.Success {
					break
				}
				time.Sleep(gcd)
			}
			if !res.Success {
				t.Fatalf("precondition: Graveyard %d refused: %s", tc.spell, e2eharness.SpellFailReasonName(res.FailReason))
			}
			time.Sleep(20 * time.Second) // the area raises undead periodically
			e2eharness.ProbeWorldAlive(t, bot, 322)
			alive := 0
			for _, e := range enemies {
				if hp, _ := bot.UnitHP(e); hp > 0 {
					alive++
				}
			}
			t.Logf("E2E_PASS: world alive 20 s after Graveyard %d at level %d (%d/%d enemies alive) (#322)", tc.spell, tc.level, alive, len(enemies))
		})
	}
}
