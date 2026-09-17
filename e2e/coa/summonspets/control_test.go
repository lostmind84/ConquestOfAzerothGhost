//go:build e2e

package summonspets_test

import (
	"testing"
	"time"

	"github.com/azerothcore/AzerothGhost/e2e/e2eharness"
)

// Control for #219: the react state and pet bar a stock warlock Imp gets on its first summon.
//
//	go test -tags=e2e ./e2e/coa/summonspets -run WarlockImpControl -count=1 -v
func TestControl_WarlockImpReactState(t *testing.T) {
	const spellSummonImp uint32 = 688
	bot := newBot(t, "WlImp", e2eharness.RaceOrc, 9, 11)
	bot.Learn(t, spellSummonImp)
	waitKnows(t, bot, spellSummonImp)
	bot.CombatReadyFull(t)
	bars := watchPetSpells(t, bot)
	// Summon Imp has a 10 second cast.
	if res := bot.Cast(t, spellSummonImp, 0, 15*time.Second); !res.Success {
		t.Fatalf("precondition: Summon Imp refused: %s", e2eharness.SpellFailReasonName(res.FailReason))
	}
	pet := bot.WaitPlayerPet(t, 15*time.Second)
	time.Sleep(2 * time.Second)
	ev, ok := bars.forPet(pet)
	t.Logf("E2E_MEASURE: warlock Imp pet bar sent %v, react %d, command %d, spells %v", ok, ev.react, ev.command, ev.spells)
}

// Control for #252: the XP the same Knight of Xoroth gets for killing the same boar itself.
//
//	go test -tags=e2e ./e2e/coa/summonspets -run OwnerKillXPControl -count=1 -v
func TestControl_KnightOwnerKillXP(t *testing.T) {
	bot := newBot(t, "KxXpC", e2eharness.RaceOrc, classKnightXoroth, 12)
	bot.CombatReadyFull(t)
	boar := spawnTarget(t, bot, creatureMottledBoar, 11, 0)
	bot.GM(t, ".gm off") // GM mode gets no kill experience
	t.Cleanup(func() { bot.GM(t, ".gm on") })
	before := bot.PlayerXP()
	bot.Engage(t, boar, 10*time.Second)
	bot.GM(t, ".damage 5000")
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline) && !unitDead(bot, boar); {
		time.Sleep(250 * time.Millisecond)
	}
	time.Sleep(2 * time.Second)
	t.Logf("E2E_MEASURE: owner kill of a level 11 boar at level 12: XP %d -> %d", before, bot.PlayerXP())
}
