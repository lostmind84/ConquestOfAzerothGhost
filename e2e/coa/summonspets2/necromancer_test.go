//go:build e2e

package summonspets2_test

import (
	"bytes"
	"encoding/binary"
	"sync"
	"testing"
	"time"

	"github.com/azerothcore/AzerothGhost/e2e/e2eharness"
)

const (
	spellGhoulOccupancy   uint32 = 805019 // "A Ghoul is currently occupying 1 Life Force."
	spellRaiseGhoul       uint32 = 500971 // creature 50073, 1 Life Force
	spellCommandGhouls    uint32 = 504021
	spellGhoulCommand     uint32 = 801514 // cast by each Ghoul: damage, crit debuff, heal to the master
	spellGhoulFrenzyHeal  uint32 = 707000 // "Heals the Necromancer."
	spellGhoulPassiveHeal uint32 = 805290 // proc aura that triggers 707000
	creatureGhoul         uint32 = 50073
	smsgSpellHealLog      uint16 = 0x0150
	ghoulLevel                   = 19
)

// healEvent is one SMSG_SPELLHEALLOG.
type healEvent struct {
	target  uint64
	caster  uint64
	spellID uint32
	amount  uint32
}

type healLog struct {
	mu     sync.Mutex
	events []healEvent
}

func watchHeals(t *testing.T, bot *e2eharness.ScenarioBot) *healLog {
	t.Helper()
	log := &healLog{}
	cancel := bot.World.AddPacketHook(func(opcode uint16, data []byte) {
		if opcode != smsgSpellHealLog {
			return
		}
		r := bytes.NewReader(data)
		target := readPackedGUID(r)
		caster := readPackedGUID(r)
		var body struct {
			SpellID uint32
			Amount  uint32
		}
		if binary.Read(r, binary.LittleEndian, &body) != nil {
			return
		}
		log.mu.Lock()
		log.events = append(log.events, healEvent{target, caster, body.SpellID, body.Amount})
		log.mu.Unlock()
	})
	t.Cleanup(cancel)
	return log
}

func (l *healLog) snapshot() []healEvent {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]healEvent(nil), l.events...)
}

// necromancerWithGhouls raises n Ghouls next to a hostile, non-moving dummy creature and sends them at it.
func necromancerWithGhouls(t *testing.T, prefix string, level, n int) (*e2eharness.ScenarioBot, []uint64, uint64) {
	t.Helper()
	bot := newBot(t, prefix, e2eharness.RaceUndead, classNecromancer, level)
	for _, spell := range []uint32{spellRaiseGhoul, spellCommandGhouls} {
		if !bot.World.KnowsSpell(spell) {
			bot.Learn(t, spell)
			waitKnows(t, bot, spell)
		}
	}
	bot.CombatReadyFull(t)
	for i := 0; i < n; i++ {
		if res := bot.Cast(t, spellRaiseGhoul, 0, 10*time.Second); !res.Success {
			t.Fatalf("precondition: raise %d refused: %s", i+1, e2eharness.SpellFailReasonName(res.FailReason))
		}
		time.Sleep(gcd)
	}
	var ghouls []uint64
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline) && len(ghouls) < n; time.Sleep(250 * time.Millisecond) {
		ghouls = ownedUnits(bot, creatureGhoul)
	}
	if len(ghouls) < n {
		t.Fatalf("precondition: %d of %d ghouls visible", len(ghouls), n)
	}
	target := spawnTarget(t, bot, creatureMottledBoar, level, factionHostile)
	bot.GM(t, ".npc set react 0")
	// Minion spells that target their master skip an invisible game master, which players never are.
	bot.GM(t, ".gm off")
	t.Cleanup(func() { bot.GM(t, ".gm on") })
	bot.Attack(t, target)
	return bot, ghouls, target
}

// Main project issue #1455: a Ghoul's auto attacks do not heal the Necromancer (Raise: Ghoul tooltip).
//
//	go test -tags=e2e ./e2e/coa/summonspets2 -run GhoulAutoAttackHeals -count=1 -v
func TestNecromancer_GhoulAutoAttackHeals(t *testing.T) {
	bot, ghouls, target := necromancerWithGhouls(t, "NcGhHl", ghoulLevel, 1)
	heals := watchHeals(t, bot)
	ghoul := ghouls[0]
	engaged := false
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(250 * time.Millisecond) {
		if bot.UnitTarget(ghoul) == target {
			engaged = true
			break
		}
	}
	if !engaged {
		t.Fatalf("precondition: the Ghoul never attacked the target")
	}
	time.Sleep(10 * time.Second)
	for _, ev := range heals.snapshot() {
		if ev.caster == ghoul && ev.target == bot.GUID {
			t.Logf("E2E_PASS: the Ghoul healed the Necromancer (spell %d, %d) (#1455)", ev.spellID, ev.amount)
			return
		}
	}
	if unitDead(bot, target) {
		t.Fatalf("precondition: the target died before the check")
	}
	t.Logf("Ghoul Passive Healing %d: on the Ghoul %v, on the Necromancer %v", spellGhoulPassiveHeal,
		bot.UnitHasAura(ghoul, spellGhoulPassiveHeal), bot.HasAura(spellGhoulPassiveHeal))
	t.Errorf("E2E_FAIL: 10 s of Ghoul auto attacks healed the Necromancer 0 times; heals seen %v (#1455)", heals.snapshot())
}

// Main project issue #1456: with more than 2 Ghouls on a target, Command: Ghouls deals damage and heals only twice.
//
//	go test -tags=e2e ./e2e/coa/summonspets2 -run CommandGhoulsEachGhoul -count=1 -v
func TestNecromancer_CommandGhoulsEachGhoul(t *testing.T) {
	const ghoulCount = 3 // the report had 4; a level 19 Necromancer without talents holds 3
	bot, ghouls, target := necromancerWithGhouls(t, "NcGhCm", ghoulLevel, ghoulCount)
	damage := watchSpellDamage(t, bot, spellGhoulCommand)
	heals := watchHeals(t, bot)
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(250 * time.Millisecond) {
		near := 0
		for _, g := range ghouls {
			if bot.UnitTarget(g) == target && distBetween(bot, g, target) < 5 {
				near++
			}
		}
		if near == ghoulCount {
			break
		}
	}
	for _, g := range ghouls {
		t.Logf("ghoul 0x%X target 0x%X, %.1f yd from the target", g, bot.UnitTarget(g), distBetween(bot, g, target))
	}
	_ = bot.World.SetTarget(target)
	if res := castLanded(t, bot, spellCommandGhouls, target, 3); !res.Success {
		t.Fatalf("precondition: Command: Ghouls refused: %s", e2eharness.SpellFailReasonName(res.FailReason))
	}
	time.Sleep(2 * time.Second)
	hits, healed := map[uint64]bool{}, map[uint64]bool{}
	for _, ev := range damage.snapshot() {
		if ev.target == target {
			hits[ev.attacker] = true
		}
	}
	for _, ev := range heals.snapshot() {
		if ev.spellID == spellGhoulCommand && ev.target == bot.GUID {
			healed[ev.caster] = true
		}
	}
	t.Logf("Command: Ghouls with %d ghouls: %d damaged the target, %d healed the Necromancer; damage %v heals %v", ghoulCount, len(hits), len(healed), damage.snapshot(), heals.snapshot())
	if len(hits) < ghoulCount || len(healed) < ghoulCount {
		t.Errorf("E2E_FAIL: Command: Ghouls hit with %d and healed with %d of %d Ghouls (#1456)", len(hits), len(healed), ghoulCount)
		return
	}
	t.Logf("E2E_PASS: every Ghoul expelled plague and healed the Necromancer (#1456)")
}

// Main project issue #141: each raised minion shows a buff on the Necromancer, and right-clicking that buff
// dismisses the minion. The buff and the dismiss were missing.
//
//	go test -tags=e2e ./e2e/coa/summonspets2 -run CancelMinionBuffDismisses -count=1 -v
func TestNecromancer_CancelMinionBuffDismisses(t *testing.T) {
	bot, ghouls, _ := necromancerWithGhouls(t, "NcDism", ghoulLevel, 2)
	visible := false
	for deadline := time.Now().Add(3 * time.Second); time.Now().Before(deadline) && !visible; time.Sleep(100 * time.Millisecond) {
		visible = bot.HasAura(spellGhoulOccupancy)
	}
	if !visible {
		t.Fatalf("E2E_FAIL: no Ghoul buff (%d) on the Necromancer with 2 Ghouls raised (#141)", spellGhoulOccupancy)
	}
	bot.CancelAura(t, spellGhoulOccupancy)
	time.Sleep(2 * time.Second)
	alive := 0
	for _, g := range ghouls {
		if !unitDead(bot, g) {
			alive++
		}
	}
	t.Logf("after cancelling one Ghoul buff: %d of 2 Ghouls remain, buff %v", alive, bot.HasAura(spellGhoulOccupancy))
	switch {
	case alive == 2:
		t.Errorf("E2E_FAIL: cancelling the Ghoul buff dismissed no Ghoul (#141)")
	case alive == 0:
		t.Errorf("E2E_FAIL: cancelling one Ghoul buff dismissed both Ghouls (#141)")
	case !bot.HasAura(spellGhoulOccupancy):
		t.Errorf("E2E_FAIL: the remaining Ghoul lost its buff (#141)")
	default:
		t.Logf("E2E_PASS: cancelling a Ghoul buff dismissed that Ghoul only (#141)")
	}
}
