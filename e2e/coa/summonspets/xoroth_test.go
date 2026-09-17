//go:build e2e

package summonspets_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/azerothcore/AzerothGhost/e2e/e2eharness"
)

const (
	specXorothImps            uint32 = 16 // Greater Imp and Impish Pestilence tree
	specXorothHellfire        uint32 = 17 // Sacrificial Circle tree
	talentGreaterImp          uint32 = 4017
	spellGreaterImpTalent     uint32 = 92101
	spellSummonGreaterImp     uint32 = 520661 // SUMMON_PET creature 510100
	talentImpishPestilence    uint32 = 4706
	spellImpishPestilence     uint32 = 300398
	talentPestApocalypse      uint32 = 30697
	spellPestilenceApocalypse uint32 = 804786
	spellPestilenceWar        uint32 = 802345
	spellPetPestApocalypse    uint32 = 806962
	spellPetPestWar           uint32 = 802604
	talentSacrificialCircle   uint32 = 6392
	spellSacrificialCircle    uint32 = 805677
	spellHellfireImpSummon    uint32 = 805966 // summons creature 50301
	creatureHellfireImp       uint32 = 50301
	creatureGreaterImp        uint32 = 510100
)

func greaterImp(t *testing.T, bot *e2eharness.ScenarioBot) uint64 {
	t.Helper()
	bot.SetSpecialization(t, specXorothImps)
	bot.SetTalentRank(t, talentGreaterImp, 1)
	waitKnows(t, bot, spellSummonGreaterImp)
	bot.CombatReadyFull(t)
	if res := bot.Cast(t, spellSummonGreaterImp, 0, 15*time.Second); !res.Success {
		t.Fatalf("precondition: Summon: Greater Imp refused: %s", e2eharness.SpellFailReasonName(res.FailReason))
	}
	pet := bot.WaitPlayerPet(t, 10*time.Second)
	bot.WaitUnitGUID(t, pet, 5*time.Second)
	return pet
}

// Main project issue #252: a Knight of Xoroth gets no experience when the Greater Imp kills a creature alone.
// Upstream AzerothCore denies experience and loot for creatures soloed by a pet (azerothcore#11969, #25676),
// so this only records the behaviour; TestControl_KnightOwnerKillXP measures the owner's own kill.
//
//	go test -tags=e2e ./e2e/coa/summonspets -run GreaterImpKillXP -count=1 -v
func TestKnightOfXoroth_GreaterImpKillXP(t *testing.T) {
	bot := newBot(t, "KxXp", e2eharness.RaceOrc, classKnightXoroth, 12)
	pet := greaterImp(t, bot)
	bot.GM(t, ".cheat god off")
	boar := spawnTarget(t, bot, creatureMottledBoar, 11, 0)
	bot.GM(t, ".gm off") // GM mode gets no kill experience
	t.Cleanup(func() { bot.GM(t, ".gm on") })
	before := bot.PlayerXP()
	bot.PetAttack(t, boar)
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) && !unitDead(bot, boar) {
		time.Sleep(500 * time.Millisecond)
	}
	if !unitDead(bot, boar) {
		t.Fatalf("precondition: the Greater Imp did not kill the boar in 60 s (pet target 0x%X)", bot.UnitTarget(pet))
	}
	time.Sleep(2 * time.Second)
	if after := bot.PlayerXP(); after > before {
		t.Logf("E2E_MEASURE: the imp's solo kill gave %d XP (#252)", after-before)
		return
	}
	t.Logf("E2E_MEASURE: no XP after the Greater Imp killed a level 11 boar alone, as upstream intends (#252)")
}

// Main project issue #375: with Impish Pestilence the Greater Imp keeps a Lesser Pestilence that does not match the
// Knight's current Pestilence after switching Apocalypse -> War -> Apocalypse. The talent text limits the pet
// application to once every 10 seconds, so the switches here wait longer than that.
//
//	go test -tags=e2e ./e2e/coa/summonspets -run ImpishPestilence -count=1 -v
func TestKnightOfXoroth_ImpishPestilenceFollowsSwitch(t *testing.T) {
	bot := newBot(t, "KxPest", e2eharness.RaceOrc, classKnightXoroth, 42)
	pet := greaterImp(t, bot)
	for _, talent := range [][2]uint32{{talentImpishPestilence, spellImpishPestilence}, {talentPestApocalypse, spellPestilenceApocalypse}} {
		if !bot.World.KnowsSpell(talent[1]) {
			bot.SetTalentRank(t, talent[0], 1)
			waitKnows(t, bot, talent[1])
		}
	}
	bot.Learn(t, spellPestilenceWar)
	check := func(step string, cast, want uint32) {
		t.Helper()
		if res := castLanded(t, bot, cast, 0, 3); !res.Success {
			t.Fatalf("precondition: Pestilence %d refused: %s", cast, e2eharness.SpellFailReasonName(res.FailReason))
		}
		time.Sleep(settle)
		t.Logf("%s: player aura %d %v, pet Apocalypse %v, pet War %v", step, cast, bot.HasAura(cast),
			bot.UnitHasAura(pet, spellPetPestApocalypse), bot.UnitHasAura(pet, spellPetPestWar))
		if !bot.UnitHasAura(pet, want) {
			t.Errorf("E2E_FAIL: %s: Greater Imp lacks Lesser Pestilence %d (#375)", step, want)
		}
	}
	check("first Apocalypse", spellPestilenceApocalypse, spellPetPestApocalypse)
	time.Sleep(11 * time.Second)
	check("then War", spellPestilenceWar, spellPetPestWar)
	time.Sleep(11 * time.Second)
	check("back to Apocalypse", spellPestilenceApocalypse, spellPetPestApocalypse)
}

// Main project issue #209 (and #334): Sacrificial Circle does nothing with a Hellfire Imp out.
//
//	go test -tags=e2e ./e2e/coa/summonspets -run SacrificialCircle -count=1 -v
func TestKnightOfXoroth_SacrificialCircle(t *testing.T) {
	sacrificialCircle(t, "KxSac")
}

func sacrificialCircle(t *testing.T, prefix string) {
	bot := newBot(t, prefix, e2eharness.RaceOrc, classKnightXoroth, 29)
	bot.SetSpecialization(t, specXorothHellfire)
	if !bot.World.KnowsSpell(spellSacrificialCircle) {
		bot.SetTalentRank(t, talentSacrificialCircle, 1)
		waitKnows(t, bot, spellSacrificialCircle)
	}
	bot.CombatReadyFull(t)
	bot.GM(t, ".cheat god off")
	bot.CastSelfGM(t, spellHellfireImpSummon)
	var imps []uint64
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline) && len(imps) == 0; time.Sleep(250 * time.Millisecond) {
		imps = ownedUnits(bot, creatureHellfireImp)
	}
	if len(imps) == 0 {
		t.Fatalf("precondition: no Hellfire Imp appeared after casting %d", spellHellfireImpSummon)
	}
	_ = bot.World.SetTarget(bot.GUID)
	bot.GM(t, ".damage 500")
	time.Sleep(settle)
	hpBefore, maxHP := bot.UnitHP(bot.GUID)
	res, err := bot.TryCast(t, spellSacrificialCircle, 0, castTimeout)
	if err != nil {
		t.Fatalf("Sacrificial Circle: %v", err)
	}
	if !res.Success {
		t.Errorf("E2E_FAIL: Sacrificial Circle refused with %d Hellfire Imp(s) out: %s (#209)", len(imps),
			e2eharness.SpellFailReasonName(res.FailReason))
		return
	}
	time.Sleep(2 * time.Second)
	hpAfter, _ := bot.UnitHP(bot.GUID)
	alive := len(ownedUnits(bot, creatureHellfireImp))
	t.Logf("health %d -> %d of %d, living imps %d -> %d", hpBefore, hpAfter, maxHP, len(imps), alive)
	if alive < len(imps) && hpAfter > hpBefore {
		t.Logf("E2E_PASS: Sacrificial Circle sacrificed an imp and healed (#209)")
		return
	}
	t.Errorf("E2E_FAIL: %s (#209)", fmt.Sprintf("Sacrificial Circle cast but imps %d -> %d, health %d -> %d",
		len(imps), alive, hpBefore, hpAfter))
}
