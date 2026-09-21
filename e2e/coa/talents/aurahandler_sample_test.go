//go:build e2e

package talents_test

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/azerothcore/AzerothGhost/e2e/e2eharness"
)

// Sample of the generated "Spell Script / Aura Handler Not Implemented" reports (3120 issues filed from a grep
// of spell IDs in the C++ sources). A static triage against Spell.dbc split them into "generic" (every effect runs
// through a core handler), "unbound proc" (a proc aura with no ProcFlags and no spell_proc row) and "unbound dummy"
// (a dummy effect or aura that nothing binds). Each test drives the tooltip's mechanic once to measure how many
// reports of each group describe a real defect.
//
//	go test -tags=e2e ./e2e/coa/talents -run AuraHandlerSample -count=1 -v -p 1

const (
	smsgForceRunSpeedChange uint16 = 0x00E2
	smsgPeriodicAuraLog     uint16 = 0x024E
	unitFieldPowerRunic     uint16 = 0x0006 + 0x0013 + 6 // UNIT_FIELD_POWER7

	itemWornShortbow uint32 = 2504
)

// powers formats UNIT_FIELD_POWER1..7 / UNIT_FIELD_MAXPOWER1..7.
func powers(bot *e2eharness.ScenarioBot) string {
	out := ""
	for i := uint16(0); i < 7; i++ {
		out += fmt.Sprintf(" p%d=%d/%d", i, selfValue(bot, 0x0006+0x0013+i), selfValue(bot, 0x0006+0x001A+i))
	}
	return out
}

// stepBack moves the bot away from the target and faces it again (ranged spells refuse TOO_CLOSE).
func stepBack(t *testing.T, bot *e2eharness.ScenarioBot, target uint64, yards float32) {
	t.Helper()
	x, y, z, mapID := bot.Pos()
	bot.Teleport(t, x+yards, y, z, mapID)
	time.Sleep(settle)
	bot.Face(t, target)
}

// sampleTalent selects the specialization when needed, takes the talent and waits for its spell.
func sampleTalent(t *testing.T, bot *e2eharness.ScenarioBot, spec, entry, spell uint32) {
	t.Helper()
	if spec != 0 {
		bot.SetSpecialization(t, spec)
	}
	bot.SetTalentRank(t, entry, 1)
	if !waitSpell(bot, spell, 5*time.Second) {
		t.Fatalf("precondition: talent spell %d not learned after .localtalent %d 1", spell, entry)
	}
}

// waitUnitAura polls a unit's aura without failing the test.
func waitUnitAura(bot *e2eharness.ScenarioBot, guid uint64, spellID uint32, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if bot.UnitHasAura(guid, spellID) {
			return true
		}
		time.Sleep(50 * time.Millisecond)
	}
	return bot.UnitHasAura(guid, spellID)
}

// Generic: Soul Tap (807397) is SPELL_EFFECT_ENERGIZE of 50 Runic Power (#3712).
func TestAuraHandlerSample_Generic_ReaperSoulTap(t *testing.T) {
	bot := newBot(t, "AhSTap", e2eharness.RaceOrc, classReaper, 60)
	sampleTalent(t, bot, 0, 29564, 807397)
	bot.CombatReadyFull(t)
	time.Sleep(settle)
	before := selfValue(bot, unitFieldPowerRunic)
	t.Logf("before Soul Tap:%s", powers(bot))
	if res := castLanded(t, bot, 807397, 0, 3); !res.Success {
		t.Fatalf("Soul Tap refused: %s", e2eharness.SpellFailReasonName(res.FailReason))
	}
	time.Sleep(settle)
	after := selfValue(bot, unitFieldPowerRunic)
	t.Logf("Soul Tap: Runic Power %d -> %d;%s", before, after, powers(bot))
	if after < before+400 {
		t.Errorf("E2E_FAIL: Soul Tap generated %d Runic Power (raw), tooltip says 50 (#3712)", int(after)-int(before))
		return
	}
	t.Logf("E2E_PASS: Soul Tap generated Runic Power (#3712 not reproduced)")
}

// Generic: Uncanny Speed (807491) is SPELL_AURA_MOD_INCREASE_SPEED 20% plus melee haste (#3677).
func TestAuraHandlerSample_Generic_BloodmageUncannySpeed(t *testing.T) {
	bot := newBot(t, "AhUncan", e2eharness.RaceHuman, classBloodmage, 60)
	var mu sync.Mutex
	var speed float32
	self := bot.World.CharGUID()
	cancel := bot.World.AddPacketHook(func(op uint16, data []byte) {
		if op != smsgForceRunSpeedChange {
			return
		}
		r := bytes.NewReader(data)
		if readPackedGUID(r) != self {
			return
		}
		var body struct {
			Counter uint32
			Unk     uint8
			Speed   float32
		}
		if binary.Read(r, binary.LittleEndian, &body) == nil {
			mu.Lock()
			speed = body.Speed
			mu.Unlock()
		}
	})
	defer cancel()
	sampleTalent(t, bot, 99, 33705, 807491)
	t.Logf("Uncanny Speed aura visible to the client: %v (passive auras can be hidden)", waitAura(bot, 807491, 3*time.Second))
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		s := speed
		mu.Unlock()
		if s > 8.3 {
			t.Logf("E2E_PASS: run speed forced to %.2f with Uncanny Speed (#3677 not reproduced)", s)
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	mu.Lock()
	defer mu.Unlock()
	t.Errorf("E2E_FAIL: Uncanny Speed aura present, last forced run speed %.2f, expected 8.40 (#3677)", speed)
}

// Generic: Neurotoxin Arrow (500071) deals damage and triggers the silence 500616 (#3735).
func TestAuraHandlerSample_Generic_RangerNeurotoxinArrow(t *testing.T) {
	bot := newBot(t, "AhNeuro", e2eharness.RaceHuman, classRanger, 60)
	sampleTalent(t, bot, 28, 34148, 500071)
	equip(t, bot, itemWornShortbow)
	dummy := spawnTarget(t, bot, e2eharness.CreatureHeroicTrainingDummy, 58)
	bot.CombatReadyFull(t)
	stepBack(t, bot, dummy, 12)
	for attempt := 1; attempt <= 5; attempt++ {
		bot.ApplyAura(t, 804329) // Advantage, Neurotoxin Arrow's caster aura
		if res := castLanded(t, bot, 500071, dummy, 3); !res.Success {
			t.Fatalf("Neurotoxin Arrow refused: %s", e2eharness.SpellFailReasonName(res.FailReason))
		}
		if waitUnitAura(bot, dummy, 500616, 2*time.Second) {
			t.Logf("E2E_PASS: Neurotoxin Arrow silenced the target on cast %d (#3735 not reproduced)", attempt)
			return
		}
		time.Sleep(gcd)
	}
	t.Errorf("E2E_FAIL: 5 Neurotoxin Arrows, no silence 500616 on the target (#3735)")
}

// Unbound proc: Darkrend Scythe (805205) should apply the bleed 704256 when Doomrend deals damage (#3497).
func TestAuraHandlerSample_Proc_ReaperDarkrendScythe(t *testing.T) {
	bot := newBot(t, "AhDkRnd", e2eharness.RaceOrc, classReaper, 60)
	bot.GM(t, ".reload spell_proc")
	sampleTalent(t, bot, 56, 30038, 805205)
	doomrend := knownSpell(t, bot, 800172)
	// A creature, not the training dummy: the bleed is mechanic 15, which training dummies may be immune to.
	dummy := spawnTarget(t, bot, creatureDefiasThug, 60)
	bot.CombatReadyFull(t)
	hits := watchSpellLog(t, bot, smsgSpellNonMeleeDamageLog, doomrend)
	ticks := watchSpellLog(t, bot, smsgPeriodicAuraLog, 704256)
	bot.ApplyAura(t, 803031) // Soul Infusion, Doomrend's caster aura (applied while the bot is still selected)
	_ = bot.World.SetTarget(dummy)
	bled := false
	for cast := 1; cast <= 8 && len(hits.snapshot()) < 5 && !bled; cast++ {
		if !bot.HasAura(803031) {
			_ = bot.World.SetTarget(bot.World.CharGUID())
			bot.ApplyAura(t, 803031)
			_ = bot.World.SetTarget(dummy)
		}
		_ = castLanded(t, bot, doomrend, dummy, 3)
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) && !bled {
			bled = bot.UnitHasAura(dummy, 704256) || len(ticks.snapshot()) > 0
			time.Sleep(100 * time.Millisecond)
		}
		t.Logf("cast %d: %d Doomrend hit(s), bleed aura %v, %d bleed tick(s)", cast, len(hits.snapshot()),
			bot.UnitHasAura(dummy, 704256), len(ticks.snapshot()))
	}
	if len(hits.snapshot()) == 0 {
		t.Fatalf("precondition: no Doomrend damage in 8 casts")
	}
	if bled {
		t.Logf("E2E_PASS: Doomrend damage applied Darkrend Scythe 704256 (#3497 not reproduced)")
		return
	}
	t.Errorf("E2E_FAIL: %d Doomrend hits with Darkrend Scythe learned, no bleed 704256 on the target (#3497)",
		len(hits.snapshot()))
}

// Unbound proc: The Jailer's Call (805195) should add 704299 damage to attacks on targets below 20% health (#3491).
func TestAuraHandlerSample_Proc_ReaperJailersCall(t *testing.T) {
	bot := newBot(t, "AhJail", e2eharness.RaceOrc, classReaper, 60)
	sampleTalent(t, bot, 55, 30536, 805195)
	dummy := spawnTarget(t, bot, creatureDefiasThug, 60)
	time.Sleep(settle)
	bot.CombatReadyFull(t)
	if _, max := bot.UnitHP(dummy); max > 0 {
		bot.Damage(t, dummy, max*85/100)
	}
	time.Sleep(settle)
	hp, max := bot.UnitHP(dummy)
	t.Logf("dummy health after .damage: %d / %d", hp, max)
	if max == 0 || hp == 0 || hp*5 > max {
		t.Fatalf("precondition: dummy not alive below 20%% health (%d / %d)", hp, max)
	}
	extra := watchSpellLog(t, bot, smsgSpellNonMeleeDamageLog, 704299)
	swings := watchSwings(t, bot)
	bot.Attack(t, dummy)
	deadline := time.Now().Add(12 * time.Second)
	for time.Now().Before(deadline) && len(extra.snapshot()) == 0 {
		time.Sleep(200 * time.Millisecond)
	}
	hp, _ = bot.UnitHP(dummy)
	t.Logf("%d swings, %d Jailer's Call hits, dummy health %d", swings.swings(), len(extra.snapshot()), hp)
	if swings.swings() == 0 {
		t.Fatalf("precondition: no melee swing on the dummy")
	}
	if len(extra.snapshot()) == 0 {
		t.Errorf("E2E_FAIL: %d swings on a target below 20%% health, no 704299 damage (#3491)", swings.swings())
		return
	}
	t.Logf("E2E_PASS: The Jailer's Call added 704299 damage (#3491 not reproduced)")
}

// Unbound proc: Shaman Training (805445) should apply Bioerosion 575848 when Wildclaw deals damage (#3519).
func TestAuraHandlerSample_Proc_PrimalistShamanTraining(t *testing.T) {
	bot := newBot(t, "AhShTr", e2eharness.RaceTauren, classPrimalist, 60)
	sampleTalent(t, bot, 58, 30967, 805445)
	wildclaw := knownSpell(t, bot, 520566, 520565, 520564, 520563, 520562, 520561, 520560, 800140)
	dummy := spawnTarget(t, bot, e2eharness.CreatureHeroicTrainingDummy, 58)
	bot.CombatReadyFull(t)
	hits := watchSpellLog(t, bot, smsgSpellNonMeleeDamageLog, wildclaw)
	defer func() { t.Logf("Wildclaw damage logs: %d", len(hits.snapshot())) }()
	for attempt := 1; attempt <= 5; attempt++ {
		bot.GM(t, ".modify rage 1000")
		if res := castLanded(t, bot, wildclaw, dummy, 3); !res.Success {
			t.Fatalf("Wildclaw %d refused: %s", wildclaw, e2eharness.SpellFailReasonName(res.FailReason))
		}
		if waitUnitAura(bot, dummy, 575848, 2*time.Second) {
			t.Logf("E2E_PASS: Wildclaw applied Bioerosion 575848 on cast %d (#3519 not reproduced)", attempt)
			return
		}
		time.Sleep(gcd)
	}
	t.Errorf("E2E_FAIL: 5 Wildclaw casts with Shaman Training learned, no Bioerosion 575848 on the target (#3519)")
}

// Unbound dummy: Blood Mark (707624) should put 707625 (+Physical damage taken) on enemies near the Bloodmage (#3143).
func TestAuraHandlerSample_Dummy_BloodmageBloodMark(t *testing.T) {
	bot := newBot(t, "AhBMark", e2eharness.RaceHuman, classBloodmage, 60)
	sampleTalent(t, bot, 99, 13706, 707624)
	dummy := spawnTarget(t, bot, e2eharness.CreatureHeroicTrainingDummy, 58)
	bot.CombatReadyFull(t)
	bot.Attack(t, dummy)
	if waitUnitAura(bot, dummy, 707625, 8*time.Second) {
		t.Logf("E2E_PASS: enemy near the Bloodmage has Blood Mark 707625 (#3143 not reproduced)")
		return
	}
	t.Errorf("E2E_FAIL: Blood Mark learned, no 707625 on an enemy in melee range after 8 s (self aura 707625: %v) (#3143)",
		bot.HasAura(707625))
}

// knownSpell returns the first of ids the bot knows, learning the last one when it knows none.
func knownSpell(t *testing.T, bot *e2eharness.ScenarioBot, ids ...uint32) uint32 {
	t.Helper()
	for _, id := range ids {
		if bot.World.KnowsSpell(id) {
			return id
		}
	}
	id := ids[len(ids)-1]
	bot.Learn(t, id)
	if !waitSpell(bot, id, 3*time.Second) {
		t.Fatalf("precondition: spell %d not learned", id)
	}
	return id
}
