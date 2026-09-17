//go:build e2e

package talents_test

import (
	"fmt"
	"math"
	"sync/atomic"
	"testing"
	"time"

	"github.com/azerothcore/AzerothGhost/e2e/e2eharness"
)

const (
	// Vow of Light: direct damage or healing taken grants 1 Solar Power and heals the nearest ally (505197) for 10%
	// (807750) of the amount.
	spellVowOfLight     uint32 = 807547
	spellVowOfLightHeal uint32 = 505197
	auraSolarPower      uint32 = 500149
)

var radiantFlameRanks = []uint32{806060, 560862, 560863, 560864, 560865, 560866, 560867}

const (
	// Radiant Flame: a 4 s channel (30 yd) whose aura triggers 807058 on the channel target every second.
	spellRadiantFlameTick uint32 = 807058
	smsgSpellFailure      uint16 = 0x0133
)

// Main project issue #1438: Radiant Flame starts channeling at its maximum range, then stops.
//
//	go test -tags=e2e ./e2e/coa/talents -run RadiantFlame -count=1 -v
func TestSunCleric_RadiantFlameAtMaximumRange(t *testing.T) {
	bot := newBot(t, "ScRadi", e2eharness.RaceHuman, classSunCleric, 60)
	flame := highestKnown(t, bot, radiantFlameRanks)
	x, y, z, mapID := bot.Pos()
	dummy := spawnTarget(t, bot, creatureExpertsTrainingDummy, 60)
	ticks := watchSpellLog(t, bot, smsgSpellNonMeleeDamageLog, spellRadiantFlameTick)
	var failures atomic.Int32
	cancel := bot.World.AddPacketHook(func(op uint16, data []byte) {
		if op == smsgSpellFailure {
			failures.Add(1)
		}
	})
	t.Cleanup(cancel)
	failed := false
	for _, distance := range []float32{10, 30, 32, 33} {
		bot.Teleport(t, x+distance, y, z, mapID)
		bot.CombatReadyFull(t)
		bot.GM(t, ".cheat cooldown on")
		bot.Face(t, dummy)
		time.Sleep(settle)
		ticks.reset()
		failures.Store(0)
		res, err := bot.TryCast(t, flame, dummy, castTimeout)
		if err != nil || !res.Success {
			t.Logf("%.0f yd: Radiant Flame %d refused (%v %s)", distance, flame, err,
				e2eharness.SpellFailReasonName(res.FailReason))
			continue
		}
		time.Sleep(5 * time.Second)
		dx, dy, _, _ := bot.Pos()
		got := len(ticks.snapshot())
		msg := fmt.Sprintf("%.0f yd (bot at %.1f yd from spawn): channel accepted, %d damage ticks, %d spell failures",
			distance, math.Hypot(float64(dx-x), float64(dy-y)), got, failures.Load())
		t.Log(msg)
		if got < 3 {
			failed = true
			t.Errorf("E2E_FAIL: %s (#1438)", msg)
		}
	}
	if !failed {
		t.Logf("E2E_PASS: every accepted Radiant Flame channel kept ticking")
	}
}

const (
	// Sunwell (#1440): the talent spell 560123; 560124 "Sun Well - Visual" periodically triggers the dispel 853225.
	spellSunwell       uint32 = 560123
	talentSunwell      uint32 = 30814
	spellSunwellVisual uint32 = 560124
	specSunClericPiety uint32 = 46
)

// Main project issue #1440: Sunwell no longer shows its golden effect. The client draws the effect, so this only
// records which Sunwell auras the server applies.
//
//	go test -tags=e2e ./e2e/coa/talents -run Sunwell -count=1 -v
func TestSunCleric_SunwellAuras(t *testing.T) {
	bot := newBot(t, "ScWell", e2eharness.RaceHuman, classSunCleric, 60)
	takeTalent(t, bot, specSunClericPiety, talentSunwell, spellSunwell)
	bot.CombatReadyFull(t)
	if res := castLanded(t, bot, spellSunwell, 0, 3); !res.Success {
		t.Fatalf("Sunwell refused: %s", e2eharness.SpellFailReasonName(res.FailReason))
	}
	time.Sleep(settle)
	auras := serverAuras(t, bot, bot.World.CharGUID())
	_, well := auras[spellSunwell]
	_, visual := auras[spellSunwellVisual]
	t.Logf("server auras: Sunwell %v, Sun Well - Visual %v; client: %v, %v", well, visual, bot.HasAura(spellSunwell),
		bot.HasAura(spellSunwellVisual))
	if !well {
		t.Errorf("E2E_FAIL: no Sunwell aura after the cast (#1440)")
	}
}

var illuminationRanks = []uint32{500143, 502421, 502422, 502423, 502424, 502425, 502426}

// Main project issue #1511: Vow of Light has no effect. The trigger here is direct healing taken: the cleric heals
// itself with Illumination after `.damage`, which is deterministic where a creature's melee hits are not.
//
//	go test -tags=e2e ./e2e/coa/talents -run VowOfLight -count=1 -v
func TestSunCleric_VowOfLightOnHealingTaken(t *testing.T) {
	bots := e2eharness.NewScenario(t, e2eharness.ScenarioOpts{
		Prefix: "ScVow",
		Bots: []e2eharness.BotSpec{
			{Role: "cleric", Race: e2eharness.RaceHuman, Class: classSunCleric, Level: 37},
			{Role: "ally", Race: e2eharness.RaceHuman, Class: e2eharness.ClassWarrior, Level: 37},
		},
	})
	cleric, ally := bots[0], bots[1]
	e2eharness.FormPartyAtPad(t, e2eharness.PackagePad(t), cleric, ally)
	t.Cleanup(func() { cleric.CleanupOwnedSummons(t) })
	if !cleric.World.KnowsSpell(spellVowOfLight) {
		t.Fatalf("precondition: level 37 Sun Cleric does not know Vow of Light %d", spellVowOfLight)
	}
	heal := highestKnown(t, cleric, illuminationRanks)
	cleric.CombatReadyFull(t)
	ally.GM(t, ".gm off")
	if res := castLanded(t, cleric, spellVowOfLight, 0, 3); !res.Success {
		t.Fatalf("Vow of Light refused: %s", e2eharness.SpellFailReasonName(res.FailReason))
	}
	time.Sleep(settle)
	if _, ok := serverAuras(t, cleric, cleric.World.CharGUID())[spellVowOfLight]; !ok {
		t.Fatalf("E2E_FAIL: no Vow of Light aura after the cast (#1511)")
	}
	self := cleric.World.CharGUID()
	allyGUID := ally.World.CharGUID()
	heals := watchSpellLog(t, cleric, smsgSpellHealLog)
	for attempt := 1; attempt <= 3; attempt++ {
		_ = cleric.World.SetTarget(self)
		cleric.GM(t, ".cheat god off")
		cleric.GM(t, ".damage 400")
		cleric.GM(t, ".gm off")
		time.Sleep(500 * time.Millisecond)
		if res := castLanded(t, cleric, heal, self, 3); !res.Success {
			t.Fatalf("Illumination %d refused: %s", heal, e2eharness.SpellFailReasonName(res.FailReason))
		}
		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) {
			for _, ev := range heals.snapshot() {
				if ev.spellID == spellVowOfLightHeal && ev.target == allyGUID {
					solar, ok := serverAuras(t, cleric, self)[auraSolarPower]
					t.Logf("Vow of Light healed the ally for %d; Solar Power on the server %v %v, client %v", ev.amount,
						ok, solar, cleric.HasAura(auraSolarPower))
					if !ok {
						t.Errorf("E2E_FAIL: healing taken with Vow of Light granted no Solar Power (#1511)")
						return
					}
					t.Logf("E2E_PASS: healing taken made Vow of Light heal the ally and grant Solar Power")
					return
				}
			}
			time.Sleep(100 * time.Millisecond)
		}
		t.Logf("attempt %d: no %d heal on the ally; cleric heal logs %+v", attempt, spellVowOfLightHeal, heals.snapshot())
	}
	t.Errorf("E2E_FAIL: three self-heals with Vow of Light up, no %d heal on the nearby ally (Solar Power %v) (#1511)",
		spellVowOfLightHeal, cleric.HasAura(auraSolarPower))
}
