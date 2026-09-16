//go:build e2e

package summonspets_test

import (
	"testing"
	"time"

	"github.com/azerothcore/AzerothGhost/e2e/e2eharness"
)

const (
	creatureShadowpineCatlord uint32 = 16345 // summons its Ghostclaw Lynx on reset (spell 28904)
	creatureGhostclawLynx     uint32 = 16348
)

// Main project issue #232: a Shadowpine Catlord's Ghostclaw Lynx keeps fighting after the Catlord dies.
//
//	go test -tags=e2e ./e2e/coa/summonspets -run CatlordLynx -count=1 -v
func TestCreature_CatlordLynxAfterDeath(t *testing.T) {
	bot := newBot(t, "CrCat", e2eharness.RaceOrc, classKnightXoroth, 24)
	bot.CombatReadyFull(t)
	known := map[uint64]struct{}{}
	for _, u := range bot.UnitsByEntry(80, creatureGhostclawLynx) {
		known[u.GUID] = struct{}{}
	}
	catlord := spawnTarget(t, bot, creatureShadowpineCatlord, 0, 0)
	lynxes := bot.WaitNewUnits(t, known, []uint32{creatureGhostclawLynx}, 10*time.Second)
	if len(lynxes) == 0 {
		t.Fatalf("precondition: the Catlord summoned no Ghostclaw Lynx")
	}
	lynx := lynxes[0].GUID
	bot.Engage(t, catlord, 10*time.Second)
	time.Sleep(2 * time.Second)
	killUnit(t, bot, catlord)
	time.Sleep(5 * time.Second)
	if unitDead(bot, lynx) {
		t.Logf("E2E_PASS: the Ghostclaw Lynx is gone after its Catlord died (#232)")
		return
	}
	t.Errorf("E2E_FAIL: the Ghostclaw Lynx is still alive 5 s after its Catlord died (in combat %v) (#232)", bot.UnitInCombat(lynx))
}
