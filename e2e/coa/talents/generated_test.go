//go:build e2e

package talents_test

import (
	"bytes"
	"encoding/binary"
	"testing"
	"time"

	"github.com/azerothcore/AzerothGhost/e2e/e2eharness"
)

// Generated "Partial Implementation: Secondary Mechanic Unhandled" reports. Each test drives the mechanic the
// report says is missing and checks its server-side result.

const (
	specWitchDoctorShadowhunting uint32 = 4
	specWitchDoctorVoodoo        uint32 = 5

	spellFoolsPlay       uint32 = 681007
	talentFoolsPlay      uint32 = 6388
	npcFool              uint32 = 300660
	spellSoulMarionette  uint32 = 704497
	talentSoulMarionette uint32 = 6055
	npcMarionette        uint32 = 300661
	auraMarionetteStacks uint32 = 807057
	spellSerpentWard     uint32 = 500960
	spellTrueSpirit      uint32 = 802268
	talentTrueSpirit     uint32 = 6853
	auraTrueSpiritReady  uint32 = 681233
	spellShadowstalker   uint32 = 807040
	spellVoodooHunger    uint32 = 705942
	talentVoodooHunger   uint32 = 5303
	auraVoodooHungerBuff uint32 = 807213

	spellBurrowBolt      uint32 = 802269
	itemOldBlunderbuss   uint32 = 2508
	spellProficiencyGuns uint32 = 266
	creatureDefiasThug   uint32 = 38
	spellCrystalShield   uint32 = 680748
	talentCrystalShield  uint32 = 9247
	specStarcallerCore   uint32 = 100

	unitFieldStat2      uint16 = 0x0006 + 0x004E + 2 // UNIT_FIELD_STAT0 + STAT_STAMINA
	smsgMonsterMove     uint16 = 0x00DD
	splineFlagParabolic uint32 = 0x00000800
	splineFlagAnimation uint32 = 0x00400000
)

// takeTalent selects the specialization, takes the talent and waits for its spell.
func takeTalent(t *testing.T, bot *e2eharness.ScenarioBot, spec, entry, spell uint32) {
	t.Helper()
	bot.SetSpecialization(t, spec)
	bot.SetTalentRank(t, entry, 1)
	if !waitSpell(bot, spell, 5*time.Second) {
		t.Fatalf("precondition: talent spell %d not learned after .localtalent %d 1", spell, entry)
	}
}

// ownedUnits returns the bot's summons of the given entry within 60 yards.
func ownedUnits(bot *e2eharness.ScenarioBot, entry uint32) int {
	count := 0
	for _, u := range bot.UnitsByEntry(60, entry) {
		if obj := bot.World.GetObject(u.GUID); obj != nil && u.Health > 0 {
			count++
		}
	}
	return count
}

func waitOwnedUnits(bot *e2eharness.ScenarioBot, entry uint32, want int, timeout time.Duration) int {
	deadline := time.Now().Add(timeout)
	got := ownedUnits(bot, entry)
	for got < want && time.Now().Before(deadline) {
		time.Sleep(100 * time.Millisecond)
		got = ownedUnits(bot, entry)
	}
	return got
}

// Main project issue #453: Fool's Play summons no copy of the target.
//
//	go test -tags=e2e ./e2e/coa/talents -run FoolsPlay -count=1 -v
func TestWitchDoctor_FoolsPlaySummonsCopy(t *testing.T) {
	bot := newBot(t, "WdFool", e2eharness.RaceTroll, classWitchDoctor, 60)
	takeTalent(t, bot, specWitchDoctorShadowhunting, talentFoolsPlay, spellFoolsPlay)
	dummy := spawnTarget(t, bot, e2eharness.CreatureHeroicTrainingDummy, 58)
	bot.CombatReadyFull(t)
	if res := castLanded(t, bot, spellFoolsPlay, dummy, 3); !res.Success {
		t.Fatalf("Fool's Play refused: %s", e2eharness.SpellFailReasonName(res.FailReason))
	}
	if got := waitOwnedUnits(bot, npcFool, 1, 3*time.Second); got < 1 {
		t.Errorf("E2E_FAIL: Fool's Play cast, no Fool (%d) summoned (#453)", npcFool)
		return
	}
	t.Logf("E2E_PASS: Fool's Play summoned creature %d", npcFool)
}

// Main project issue #487: Soul Marionette creates no clones and applies no stacks.
//
//	go test -tags=e2e ./e2e/coa/talents -run SoulMarionette -count=1 -v
func TestWitchDoctor_SoulMarionetteClones(t *testing.T) {
	bot := newBot(t, "WdMario", e2eharness.RaceTroll, classWitchDoctor, 60)
	takeTalent(t, bot, specWitchDoctorVoodoo, talentSoulMarionette, spellSoulMarionette)
	bot.CombatReadyFull(t)
	if res := castLanded(t, bot, spellSoulMarionette, 0, 3); !res.Success {
		t.Fatalf("Soul Marionette refused: %s", e2eharness.SpellFailReasonName(res.FailReason))
	}
	clones := waitOwnedUnits(bot, npcMarionette, 5, 3*time.Second)
	stacks := 0
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) && stacks == 0 {
		stacks = bot.AuraStacks(auraMarionetteStacks)
		time.Sleep(100 * time.Millisecond)
	}
	t.Logf("Soul Marionette: %d clone(s) of %d, %d stack(s) of %d", clones, npcMarionette, stacks, auraMarionetteStacks)
	if clones < 5 || stacks == 0 {
		t.Errorf("E2E_FAIL: Soul Marionette made %d/5 clones and %d stacks (#487)", clones, stacks)
		return
	}
	t.Logf("E2E_PASS: Soul Marionette clones channel stacks on the caster")
}

// Main project issue #489: The True Spirit does not empower the next Spirit Glaive after summoning a Serpent Ward.
//
//	go test -tags=e2e ./e2e/coa/talents -run TrueSpirit -count=1 -v
func TestWitchDoctor_TrueSpiritStacksOnWard(t *testing.T) {
	bot := newBot(t, "WdTrue", e2eharness.RaceTroll, classWitchDoctor, 60)
	takeTalent(t, bot, specWitchDoctorShadowhunting, talentTrueSpirit, spellTrueSpirit)
	if !bot.World.KnowsSpell(spellSerpentWard) {
		bot.Learn(t, spellSerpentWard)
	}
	bot.CombatReadyFull(t)
	if res := castLanded(t, bot, spellSerpentWard, 0, 3); !res.Success {
		t.Fatalf("Serpent Ward refused: %s", e2eharness.SpellFailReasonName(res.FailReason))
	}
	if !waitAura(bot, auraTrueSpiritReady, 3*time.Second) {
		t.Errorf("E2E_FAIL: Serpent Ward summoned with The True Spirit, no %d buff (#489)", auraTrueSpiritReady)
		return
	}
	t.Logf("E2E_PASS: Serpent Ward granted %d (%d stack)", auraTrueSpiritReady, bot.AuraStacks(auraTrueSpiritReady))
}

// Main project issue #490: Voodoo Hunger stacks are not gained while Shadowstalker is active.
//
//	go test -tags=e2e ./e2e/coa/talents -run VoodooHunger -count=1 -v
func TestWitchDoctor_VoodooHungerStacksInShadowstalker(t *testing.T) {
	bot := newBot(t, "WdHung", e2eharness.RaceTroll, classWitchDoctor, 60)
	takeTalent(t, bot, specWitchDoctorShadowhunting, talentVoodooHunger, spellVoodooHunger)
	if !bot.World.KnowsSpell(spellShadowstalker) {
		bot.Learn(t, spellShadowstalker)
	}
	bot.CombatReadyFull(t)
	if res := castLanded(t, bot, spellShadowstalker, 0, 3); !res.Success {
		t.Fatalf("Shadowstalker refused: %s", e2eharness.SpellFailReasonName(res.FailReason))
	}
	stacks := 0
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if s := bot.AuraStacks(auraVoodooHungerBuff); s > stacks {
			stacks = s
		}
		time.Sleep(100 * time.Millisecond)
	}
	if stacks < 2 {
		t.Errorf("E2E_FAIL: %d Voodoo Hunger stack(s) after 3 s of Shadowstalker, expected one per 0.5 s (#490)", stacks)
		return
	}
	t.Logf("E2E_PASS: %d Voodoo Hunger stacks after 3 s of Shadowstalker", stacks)
}

// Main project issue #454: Burrow Bolt damages but does not pull the target.
//
//	go test -tags=e2e ./e2e/coa/talents -run BurrowBolt -count=1 -v
func TestWitchHunter_BurrowBoltPulls(t *testing.T) {
	bot := newBot(t, "WhBurr", e2eharness.RaceHuman, classWitchHunter, 20)
	bot.Learn(t, spellBurrowBolt)
	bot.Learn(t, spellProficiencyGuns)
	equip(t, bot, itemOldBlunderbuss)
	thug := bot.Spawn(t, creatureDefiasThug, 10*time.Second)
	_ = bot.World.SetTarget(thug)
	bot.GM(t, ".npc set faction 7")
	bot.GM(t, ".npc set level 18")
	x, y, z, mapID := bot.Pos()
	bot.Teleport(t, x+15, y, z, mapID)
	time.Sleep(settle)
	bot.Face(t, thug)
	bot.CombatReadyFull(t)

	jumps := make(chan [3]float32, 8)
	cancelMoves := bot.World.AddPacketHook(func(op uint16, data []byte) {
		if op != smsgMonsterMove {
			return
		}
		r := bytes.NewReader(data)
		if readPackedGUID(r) != thug {
			return
		}
		if dest, ok := jumpDestination(r); ok {
			select {
			case jumps <- dest:
			default:
			}
		}
	})
	defer cancelMoves()

	obj := bot.World.GetObject(thug)
	if obj == nil {
		t.Fatalf("precondition: target not tracked")
	}
	ux, uy, uz := obj.InterpolatedPosition()
	before := bot.DistFrom(ux, uy, uz)
	// A missed or dodged bolt applies no pull aura: reset the cooldown and shoot again.
	for attempt := 1; attempt <= 3; attempt++ {
		if res := castLanded(t, bot, spellBurrowBolt, thug, 3); !res.Success {
			t.Fatalf("Burrow Bolt refused: %s", e2eharness.SpellFailReasonName(res.FailReason))
		}
		select {
		case dest := <-jumps:
			after := bot.DistFrom(dest[0], dest[1], dest[2])
			t.Logf("Burrow Bolt: target at %.1f yd, jumps to %.1f yd from the caster", before, after)
			if !(after < before-5) {
				t.Errorf("E2E_FAIL: Burrow Bolt jump does not bring the target closer (%.1f -> %.1f yd) (#454)",
					before, after)
				return
			}
			t.Logf("E2E_PASS: Burrow Bolt pulled the target")
			return
		case <-time.After(3 * time.Second):
			t.Logf("attempt %d: no jump sent for the target within 3 s", attempt)
			_ = bot.World.SetTarget(bot.World.CharGUID()) // .cooldown applies to the selection
			bot.GM(t, ".cooldown")
			_ = bot.World.SetTarget(thug)
		}
	}
	t.Errorf("E2E_FAIL: three Burrow Bolts cast, no jump sent for the target (#454)")
}

// Main project issue #520: Crystal Shield has an unhandled "shield" mechanic. Its tooltip describes a Stamina and
// spell power passive; the generated report matched the word "Shield" in the name.
//
//	go test -tags=e2e ./e2e/coa/talents -run CrystalShield -count=1 -v
func TestStarcaller_CrystalShieldStamina(t *testing.T) {
	bot := newBot(t, "ScCrys", e2eharness.RaceHuman, classStarcaller, 30)
	bot.SetSpecialization(t, specStarcallerCore)
	time.Sleep(settle)
	bot.SetTalentRank(t, talentCrystalShield, 1)
	if !waitSpell(bot, spellCrystalShield, 5*time.Second) {
		t.Fatalf("precondition: Crystal Shield %d not known at level 30 in specialization %d", spellCrystalShield, specStarcallerCore)
	}
	with := selfValue(bot, unitFieldStat2)
	bot.GM(t, ".unlearn 680748")
	deadline := time.Now().Add(5 * time.Second)
	for bot.World.KnowsSpell(spellCrystalShield) && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	time.Sleep(settle)
	without := selfValue(bot, unitFieldStat2)
	t.Logf("Stamina without Crystal Shield %d, with %d", without, with)
	if with <= without {
		t.Errorf("E2E_FAIL: Crystal Shield did not raise Stamina (%d -> %d) (#520)", without, with)
		return
	}
	t.Logf("E2E_PASS: Crystal Shield raised Stamina %d -> %d", without, with)
}

// jumpDestination reads the destination of a parabolic SMSG_MONSTER_MOVE after the mover GUID. The harness position
// cache does not follow parabolic splines, so the jump end point is read from the packet.
func jumpDestination(r *bytes.Reader) ([3]float32, bool) {
	var head struct {
		Unk      uint8
		Start    [3]float32
		SplineID uint32
		Type     uint8
	}
	if binary.Read(r, binary.LittleEndian, &head) != nil || head.Type != 0 {
		return [3]float32{}, false
	}
	var flags, duration uint32
	if binary.Read(r, binary.LittleEndian, &flags) != nil || flags&splineFlagParabolic == 0 {
		return [3]float32{}, false
	}
	if flags&splineFlagAnimation != 0 {
		var anim struct {
			ID    uint8
			Start uint32
		}
		if binary.Read(r, binary.LittleEndian, &anim) != nil {
			return [3]float32{}, false
		}
	}
	var parabolic struct {
		Speed float32
		Start uint32
	}
	var count uint32
	var dest [3]float32
	if binary.Read(r, binary.LittleEndian, &duration) != nil ||
		binary.Read(r, binary.LittleEndian, &parabolic) != nil ||
		binary.Read(r, binary.LittleEndian, &count) != nil || count != 1 ||
		binary.Read(r, binary.LittleEndian, &dest) != nil {
		return [3]float32{}, false
	}
	return dest, true
}
