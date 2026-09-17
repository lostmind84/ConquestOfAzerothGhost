//go:build e2e

package brokenspells_test

import (
	"bytes"
	"encoding/binary"
	"sync"
	"testing"
	"time"

	"github.com/azerothcore/AzerothGhost/e2e/e2eharness"
)

// Generated Pyromancer reports whose spell has a DUMMY or SCRIPT_EFFECT effect and no handler by ID.
// Each test casts the ability as the tooltip describes and checks the effect the tooltip names.
//
//	go test -tags=e2e -p 1 ./e2e/coa/brokenspells -run TestCoA_PyromancerGenerated -count=1 -v

const (
	raceBloodElf       uint8  = 10
	pyromancerLevel           = 60
	creatureHarvestGol uint32 = 36

	spellGazeOfYsera        uint32 = 806148 // #880
	spellGazeOfYseraSleep   uint32 = 503229
	spellGraceOfAlexstrasza uint32 = 802167 // #470
	spellGraceImmunity      uint32 = 803411
	spellDragonfire         uint32 = 500129 // #2065
	spellDragonfireMana     uint32 = 503648
	spellDormant            uint32 = 800128 // #3170
	spellDormantMana        uint32 = 800129
	spellFlareBolt          uint32 = 800790
	spellBurningSpheres     uint32 = 801487 // #3759
	creatureBurningSphere   uint32 = 50380
	spellIgnite             uint32 = 800791

	smsgSpellEnergizeLog uint16 = 0x0151
)

func newPyromancer(t *testing.T, prefix string) *e2eharness.ScenarioBot {
	t.Helper()
	bot := newBot(t, prefix, raceBloodElf, classPyromancer, pyromancerLevel)
	bot.CheatGod(t)
	return bot
}

// energizeLog records SMSG_SPELLENERGIZELOG amounts the bot gives itself, per spell.
type energizeLog struct {
	mu      sync.Mutex
	amounts map[uint32][]uint32
}

func watchEnergize(t *testing.T, bot *e2eharness.ScenarioBot) *energizeLog {
	t.Helper()
	log := &energizeLog{amounts: map[uint32][]uint32{}}
	self := bot.World.CharGUID()
	cancel := bot.World.AddPacketHook(func(opcode uint16, data []byte) {
		if opcode != smsgSpellEnergizeLog {
			return
		}
		r := bytes.NewReader(data)
		readPackedGUID(r) // target
		if readPackedGUID(r) != self {
			return
		}
		var body struct {
			SpellID, PowerType, Amount uint32
		}
		if binary.Read(r, binary.LittleEndian, &body) != nil {
			return
		}
		log.mu.Lock()
		log.amounts[body.SpellID] = append(log.amounts[body.SpellID], body.Amount)
		log.mu.Unlock()
	})
	t.Cleanup(cancel)
	return log
}

func (l *energizeLog) of(spellID uint32) []uint32 {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]uint32(nil), l.amounts[spellID]...)
}

func waitUnitAura(bot *e2eharness.ScenarioBot, guid uint64, timeout time.Duration, spellIDs ...uint32) (uint32, bool) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		for _, id := range spellIDs {
			if e2eharness.UnitHasAura(bot.World, guid, id) {
				return id, true
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	return 0, false
}

// #880 Gaze of Ysera: "Set Ysera's gaze upon an enemy, putting them to sleep for 40 sec".
func TestCoA_PyromancerGenerated_GazeOfYsera(t *testing.T) {
	bot := newPyromancer(t, "PyGaze")
	bot.Learn(t, spellGazeOfYsera)
	target := spawnTarget(t, bot, creatureHarvestGol, pyromancerLevel-2)
	if res := castLanded(t, bot, spellGazeOfYsera, target, 4); !res.Success {
		e2eharness.Preconditionf(t, "Gaze of Ysera never landed")
	}
	id, ok := waitUnitAura(bot, target, 3*time.Second, spellGazeOfYseraSleep, spellGazeOfYsera)
	if !ok {
		t.Fatalf("E2E_FAIL: #880 Gaze of Ysera left no aura (%d or %d) on its target", spellGazeOfYsera, spellGazeOfYseraSleep)
	}
	time.Sleep(5 * time.Second)
	if !e2eharness.UnitHasAura(bot.World, target, id) {
		t.Fatalf("E2E_FAIL: #880 Gaze of Ysera aura %d ended within 5 sec, sleep is 40 sec", id)
	}
	t.Logf("E2E_PASS: #880 Gaze of Ysera keeps aura %d on its target after 5 sec", id)
}

// #470 Grace of Alexstrasza: grants "stun, slow and root immunity" to the caster's group.
func TestCoA_PyromancerGenerated_GraceOfAlexstrasza(t *testing.T) {
	bot := newPyromancer(t, "PyGrace")
	bot.Learn(t, spellGraceOfAlexstrasza)
	if res := castLanded(t, bot, spellGraceOfAlexstrasza, bot.World.CharGUID(), 3); !res.Success {
		e2eharness.Preconditionf(t, "Grace of Alexstrasza refused")
	}
	if _, ok := waitUnitAura(bot, bot.World.CharGUID(), 3*time.Second, spellGraceImmunity); !ok {
		t.Fatalf("E2E_FAIL: #470 Grace of Alexstrasza did not grant its immunity aura %d, auras %v",
			spellGraceImmunity, bot.World.SelfAuras())
	}
	t.Logf("E2E_PASS: #470 Grace of Alexstrasza grants %d", spellGraceImmunity)
}

// #2065 Dragonfire: "Deals Fire Damage to an enemy target, and restores 6% of your maximum Mana".
func TestCoA_PyromancerGenerated_DragonfireMana(t *testing.T) {
	bot := newPyromancer(t, "PyDfire")
	bot.Learn(t, spellDragonfire)
	energize := watchEnergize(t, bot)
	damage := watchDamage(t, bot, spellDragonfire)
	target := spawnTarget(t, bot, creatureHarvestGol, pyromancerLevel)
	for i := 0; i < 4 && len(damage.snapshot()) == 0; i++ {
		castLanded(t, bot, spellDragonfire, target, 3)
		time.Sleep(settle)
	}
	if len(damage.snapshot()) == 0 {
		e2eharness.Preconditionf(t, "Dragonfire dealt no damage")
	}
	if len(energize.of(spellDragonfireMana)) == 0 {
		t.Fatalf("E2E_FAIL: #2065 Dragonfire restored no mana (%d), energize logs %v", spellDragonfireMana, energize.amounts)
	}
	t.Logf("E2E_PASS: #2065 Dragonfire restores mana %v", energize.of(spellDragonfireMana))
}

// #3170 Dormant: "non-periodic Fire Damage regenerates 5% maximum mana".
func TestCoA_PyromancerGenerated_Dormant(t *testing.T) {
	bot := newPyromancer(t, "PyDorm")
	bot.Learn(t, spellDormant)
	if res := castLanded(t, bot, spellDormant, bot.World.CharGUID(), 3); !res.Success {
		e2eharness.Preconditionf(t, "Dormant refused")
	}
	bot.WaitAura(t, spellDormant, 3*time.Second)
	energize := watchEnergize(t, bot)
	damage := watchDamage(t, bot, spellFlareBolt)
	target := spawnTarget(t, bot, creatureHarvestGol, pyromancerLevel)
	for i := 0; i < 5 && len(damage.snapshot()) == 0; i++ {
		castLanded(t, bot, knownRank(t, bot, spellFlareBolt), target, 3)
		time.Sleep(settle)
	}
	if len(damage.snapshot()) == 0 {
		e2eharness.Preconditionf(t, "Flare Bolt dealt no damage")
	}
	if len(energize.of(spellDormantMana)) == 0 {
		t.Fatalf("E2E_FAIL: #3170 Fire damage with Dormant restored no mana (%d), energize logs %v", spellDormantMana, energize.amounts)
	}
	t.Logf("E2E_PASS: #3170 Dormant restores mana %v", energize.of(spellDormantMana))
}

// #3759 Burning Spheres: summons 3 spheres; Ignite makes them lock on and deal Fire damage over time.
func TestCoA_PyromancerGenerated_BurningSpheres(t *testing.T) {
	bot := newPyromancer(t, "PySphe")
	bot.Learn(t, spellBurningSpheres)
	known := map[uint64]struct{}{}
	for _, u := range bot.UnitsByEntry(60, creatureBurningSphere) {
		known[u.GUID] = struct{}{}
	}
	if res := castLanded(t, bot, spellBurningSpheres, bot.World.CharGUID(), 3); !res.Success {
		e2eharness.Preconditionf(t, "Burning Spheres refused")
	}
	spheres := bot.WaitNewUnits(t, known, []uint32{creatureBurningSphere}, 3*time.Second)
	if len(spheres) != 3 {
		t.Errorf("E2E_FAIL: #3759 Burning Spheres summoned %d spheres, want 3", len(spheres))
	}
	target := spawnTarget(t, bot, creatureHarvestGol, pyromancerLevel)
	damage := watchDamage(t, bot)
	castLanded(t, bot, knownRank(t, bot, spellIgnite), target, 4)
	time.Sleep(6 * time.Second)
	other := map[uint32]int{}
	for _, e := range damage.snapshot() {
		if e.target == target {
			other[e.spellID]++
		}
	}
	t.Logf("damage on target by spell: %v", other)
	delete(other, spellIgnite)
	if len(other) == 0 {
		t.Fatalf("E2E_FAIL: #3759 spheres dealt no damage after Ignite (only Ignite hit the target)")
	}
	t.Logf("E2E_PASS: #3759 spheres add damage after Ignite: %v", other)
}
