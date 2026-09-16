//go:build e2e

package talents_test

import (
	"testing"
	"time"

	"github.com/azerothcore/AzerothGhost/e2e/e2eharness"
)

const (
	unitFieldPower1 uint16 = 0x0006 + 0x0013 // UNIT_FIELD_POWER1 (mana)
	unitFieldPower4 uint16 = unitFieldPower1 + 3

	// Guardian (#93)
	spellTowerFormation  uint32 = 800317
	spellRaiseShield     uint32 = 500168
	spellDeflector       uint32 = 706336
	talentDeflector      uint32 = 34422
	spellResolve         uint32 = 706806
	talentResolve        uint32 = 6818
	itemLargeRoundShield uint32 = 2129

	// Witch Doctor (#296)
	spellSeeker  uint32 = 705880
	talentSeeker uint32 = 7092

	// Sun Cleric (#1436, #1441)
	specSunClericBurningHeat uint32 = 46
	spellBurningHeat         uint32 = 704908
	talentBurningHeat        uint32 = 34052
	auraSunRayReady          uint32 = 803238
	spellSunRay              uint32 = 504764
	spellDawnfall            uint32 = 806118

	// Necromancer (#1449)
	spellVampiricAura     uint32 = 300241
	talentVampiricAura    uint32 = 6458
	spellVampiricAuraHeal uint32 = 561095
	spellCryptSwarm       uint32 = 500965
)

var horusathBlastRanks = []uint32{500154, 502380, 502381, 502382, 502383, 502384, 502385, 572796, 572801}

// Main project issue #93: the Guardian talents Deflector and Resolve do not change Raise Shield.
// Deflector: -5 s cooldown and -10% Energy cost. Resolve: +1 s duration. Values from Spell.dbc 706336/706806.
//
//	go test -tags=e2e ./e2e/coa/talents -run DeflectorResolve -count=1 -v
func TestGuardian_DeflectorResolveModifyRaiseShield(t *testing.T) {
	bot := newBot(t, "GdDefl", e2eharness.RaceHuman, classGuardian, 20)
	equip(t, bot, itemLargeRoundShield)
	if !bot.World.KnowsSpell(spellRaiseShield) {
		bot.Learn(t, spellRaiseShield)
	}
	bot.CombatReady(t)
	if res := castLanded(t, bot, spellTowerFormation, 0, 2); !res.Success {
		t.Fatalf("Tower Formation refused: %s", e2eharness.SpellFailReasonName(res.FailReason))
	}
	time.Sleep(settle)

	// measure casts Raise Shield with full Energy and returns the Energy spent and the aura duration.
	measure := func() (spent uint32, duration time.Duration) {
		bot.GM(t, ".gm on")
		bot.GM(t, ".modify energy 100")
		bot.GM(t, ".gm off")
		bot.GM(t, ".cooldown")
		time.Sleep(settle)
		before := selfValue(bot, unitFieldPower4)
		if res := castLanded(t, bot, spellRaiseShield, 0, 2); !res.Success {
			t.Fatalf("Raise Shield refused: %s", e2eharness.SpellFailReasonName(res.FailReason))
		}
		if !waitAura(bot, spellRaiseShield, 2*time.Second) {
			t.Fatalf("precondition: no Raise Shield aura")
		}
		time.Sleep(300 * time.Millisecond)
		after := selfValue(bot, unitFieldPower4)
		duration = bot.AuraMaxDuration(spellRaiseShield)
		bot.CancelAura(t, spellRaiseShield)
		if before > after {
			spent = before - after
		}
		return spent, duration
	}
	// recastAfter reports whether Raise Shield can be cast again after wait.
	recastAfter := func(wait time.Duration) bool {
		time.Sleep(wait)
		res, err := bot.TryCast(t, spellRaiseShield, 0, castTimeout)
		if err != nil {
			t.Fatalf("Raise Shield recast: %v", err)
		}
		bot.CancelAura(t, spellRaiseShield)
		return res.Success
	}

	baseCost, baseDuration := measure()
	t.Logf("without talents: Energy %d, duration %v", baseCost, baseDuration)

	bot.SetTalentRank(t, talentDeflector, 1)
	bot.SetTalentRank(t, talentResolve, 1)
	if !waitSpell(bot, spellDeflector, 5*time.Second) || !waitSpell(bot, spellResolve, 5*time.Second) {
		t.Fatalf("precondition: Deflector %v / Resolve %v not learned", bot.World.KnowsSpell(spellDeflector),
			bot.World.KnowsSpell(spellResolve))
	}
	cost, duration := measure()
	ready := recastAfter(21 * time.Second)
	t.Logf("with Deflector and Resolve: Energy %d, duration %v, recast after 21 s: %v", cost, duration, ready)

	failed := false
	if !(cost < baseCost) {
		t.Errorf("E2E_FAIL: Deflector did not reduce Raise Shield's Energy cost (%d -> %d) (#93)", baseCost, cost)
		failed = true
	}
	if !ready {
		t.Errorf("E2E_FAIL: Deflector did not bring Raise Shield's 25 s cooldown down to 20 s (#93)")
		failed = true
	}
	if !(duration > baseDuration) {
		t.Errorf("E2E_FAIL: Resolve did not lengthen Raise Shield (%v -> %v) (#93)", baseDuration, duration)
		failed = true
	}
	if !failed {
		t.Logf("E2E_PASS: Deflector and Resolve modify Raise Shield")
	}
}

// Main project issue #296: Seeker does not reduce the mana cost of Wards, Idols and Effigies (-20%, Spell.dbc 705880).
//
//	go test -tags=e2e ./e2e/coa/talents -run Seeker -count=1 -v
func TestWitchDoctor_SeekerReducesWardCost(t *testing.T) {
	bot := newBot(t, "WdSeek", e2eharness.RaceTroll, classWitchDoctor, 20)
	if !bot.World.KnowsSpell(spellSerpentWard) {
		bot.Learn(t, spellSerpentWard)
	}
	bot.CombatReady(t)
	measure := func() uint32 {
		bot.GM(t, ".cooldown")
		time.Sleep(settle)
		before := selfValue(bot, unitFieldPower1)
		if res := castLanded(t, bot, spellSerpentWard, 0, 3); !res.Success {
			t.Fatalf("Serpent Ward refused: %s", e2eharness.SpellFailReasonName(res.FailReason))
		}
		time.Sleep(500 * time.Millisecond)
		after := selfValue(bot, unitFieldPower1)
		if before > after {
			return before - after
		}
		return 0
	}
	base := measure()
	bot.SetTalentRank(t, talentSeeker, 1)
	if !waitSpell(bot, spellSeeker, 5*time.Second) {
		t.Fatalf("precondition: Seeker not learned")
	}
	bot.GM(t, ".gm on")
	bot.GM(t, ".modify mana 100000")
	bot.GM(t, ".gm off")
	time.Sleep(settle)
	with := measure()
	t.Logf("Serpent Ward mana cost: %d without Seeker, %d with", base, with)
	if base == 0 {
		t.Fatalf("precondition: Serpent Ward cost no mana")
	}
	if !(with < base) {
		t.Errorf("E2E_FAIL: Seeker did not reduce Serpent Ward's mana cost (%d -> %d) (#296)", base, with)
		return
	}
	t.Logf("E2E_PASS: Seeker reduced Serpent Ward's mana cost %d -> %d", base, with)
}

// Main project issue #1436: Burning Heat does not turn the next Sunflare into Sun Ray after Horusath Blast.
//
//	go test -tags=e2e ./e2e/coa/talents -run BurningHeat -count=1 -v
func TestSunCleric_BurningHeatHorusathBlastGrantsSunRay(t *testing.T) {
	bot := newBot(t, "ScBurn", e2eharness.RaceHuman, classSunCleric, 60)
	takeTalent(t, bot, specSunClericBurningHeat, talentBurningHeat, spellBurningHeat)
	blast := horusathBlastRanks[len(horusathBlastRanks)-1] // rank 9, level 58
	if !bot.World.KnowsSpell(blast) {
		bot.Learn(t, blast)
	}
	dummy := spawnTarget(t, bot, e2eharness.CreatureHeroicTrainingDummy, 58)
	bot.CombatReadyFull(t)
	resetCooldowns := func() { // .cooldown applies to the selection
		_ = bot.World.SetTarget(bot.World.CharGUID())
		bot.GM(t, ".cooldown")
		time.Sleep(settle)
		_ = bot.World.SetTarget(dummy)
	}
	resetCooldowns()
	for attempt := 1; attempt <= 3; attempt++ {
		if res := castLanded(t, bot, blast, dummy, 3); !res.Success {
			t.Fatalf("Horusath Blast %d refused: %s", blast, e2eharness.SpellFailReasonName(res.FailReason))
		}
		// The 803238 marker is a hidden aura; the visible result is Sun Ray replacing Sunflare in the spellbook.
		if waitSpell(bot, spellSunRay, 2*time.Second) {
			t.Logf("E2E_PASS: Horusath Blast with Burning Heat readied Sun Ray (marker aura visible: %v)",
				bot.HasAura(auraSunRayReady))
			return
		}
		t.Logf("attempt %d: Sun Ray not learned, marker aura visible: %v", attempt, bot.HasAura(auraSunRayReady))
		resetCooldowns()
	}
	t.Errorf("E2E_FAIL: three Horusath Blasts with Burning Heat, Sun Ray %d never learned (#1436)", spellSunRay)
}

// Main project issue #1441: Dawnfall gives no buff to allies inside, the caster included (Spell.dbc 806118 effect 2:
// +15% healing received for allies).
//
//	go test -tags=e2e ./e2e/coa/talents -run Dawnfall -count=1 -v
func TestSunCleric_DawnfallBuffsAlliesInside(t *testing.T) {
	bot := newBot(t, "ScDawn", e2eharness.RaceHuman, classSunCleric, 60)
	if !bot.World.KnowsSpell(spellDawnfall) {
		bot.Learn(t, spellDawnfall)
	}
	dummy := spawnTarget(t, bot, e2eharness.CreatureHeroicTrainingDummy, 60)
	bot.CombatReadyFull(t)
	obj := bot.World.GetObject(dummy)
	if obj == nil {
		t.Fatalf("precondition: dummy not tracked")
	}
	// Centre the area between the caster and the dummy so both stand inside its 8 yd radius.
	x, y, z, _ := bot.Pos()
	dx, dy, _ := obj.InterpolatedPosition()
	cx, cy := (x+dx)/2, (y+dy)/2
	if bot.DistFrom(dx, dy, z) > 12 {
		t.Fatalf("precondition: dummy %.1f yd away, too far for one Dawnfall", bot.DistFrom(dx, dy, z))
	}
	if res := bot.CastAtPosition(t, spellDawnfall, cx, cy, z, castTimeout); !res.Success {
		t.Fatalf("Dawnfall refused: %s", e2eharness.SpellFailReasonName(res.FailReason))
	}
	self := waitAura(bot, spellDawnfall, 3*time.Second)
	enemy := bot.UnitHasAura(dummy, spellDawnfall)
	t.Logf("Dawnfall aura: caster %v, enemy dummy %v", self, enemy)
	if !enemy {
		t.Fatalf("precondition: the enemy inside Dawnfall has no %d aura either", spellDawnfall)
	}
	if !self {
		t.Errorf("E2E_FAIL: enemies inside Dawnfall get its aura, the caster standing inside does not (#1441)")
		return
	}
	t.Logf("E2E_PASS: the caster standing in Dawnfall has its aura")
}

// Main project issue #1449: Vampiric Aura heals the Necromancer from its own damage and above 50% health. The
// tooltip (300241) says Undead minions heal the Necromancer while it is below 50% health.
//
//	go test -tags=e2e ./e2e/coa/talents -run VampiricAura -count=1 -v
func TestNecromancer_VampiricAuraIgnoresOwnDamageAboveHalfHealth(t *testing.T) {
	bot := newBot(t, "NcVamp", e2eharness.RaceUndead, classNecromancer, 19)
	bot.SetTalentRank(t, talentVampiricAura, 1)
	if !waitSpell(bot, spellVampiricAura, 5*time.Second) {
		t.Fatalf("precondition: Vampiric Aura not learned")
	}
	if !bot.World.KnowsSpell(spellCryptSwarm) {
		bot.Learn(t, spellCryptSwarm)
	}
	dummy := spawnTarget(t, bot, e2eharness.CreatureHeroicTrainingDummy, 19)
	heals := watchSpellLog(t, bot, smsgSpellHealLog, spellVampiricAuraHeal)
	bot.CombatReadyFull(t)
	time.Sleep(1500 * time.Millisecond) // Vampiric Aura's periodic effect applies its proc aura every second
	if res := castLanded(t, bot, spellCryptSwarm, dummy, 3); !res.Success {
		t.Fatalf("Crypt Swarm refused: %s", e2eharness.SpellFailReasonName(res.FailReason))
	}
	bot.Attack(t, dummy)
	time.Sleep(8 * time.Second)
	if got := heals.snapshot(); len(got) > 0 {
		t.Errorf("E2E_FAIL: at full health, the Necromancer's own Crypt Swarm healed it through Vampiric Aura %d time(s) "+
			"(first %+v) (#1449)", len(got), got[0])
		return
	}
	t.Logf("E2E_PASS: no Vampiric Aura heal from the Necromancer's own damage at full health")
}
