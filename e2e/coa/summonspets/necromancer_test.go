//go:build e2e

package summonspets_test

import (
	"testing"
	"time"

	"github.com/azerothcore/AzerothGhost/e2e/e2eharness"
)

const (
	spellRaiseLesserSkeleton uint32 = 500970 // creature 50065, 1 Life Force
	spellUndeadAssault       uint32 = 500982
	spellUndeadPacify        uint32 = 500983
	spellUndeadProtect       uint32 = 500985
	spellGraveMarch          uint32 = 500991
	creatureLesserSkeleton   uint32 = 50065
	creatureRabbit           uint32 = 721 // critter
)

func necromancer(t *testing.T, prefix string, level int) *e2eharness.ScenarioBot {
	t.Helper()
	bot := newBot(t, prefix, e2eharness.RaceUndead, classNecromancer, level)
	for _, spell := range []uint32{spellRaiseLesserSkeleton, spellUndeadAssault, spellUndeadPacify, spellUndeadProtect, spellGraveMarch} {
		if !bot.World.KnowsSpell(spell) {
			bot.Learn(t, spell)
			waitKnows(t, bot, spell)
		}
	}
	bot.CombatReadyFull(t)
	return bot
}

func raiseSkeletons(t *testing.T, bot *e2eharness.ScenarioBot, n int) []uint64 {
	t.Helper()
	for i := 0; i < n; i++ {
		if res := bot.Cast(t, spellRaiseLesserSkeleton, 0, 10*time.Second); !res.Success {
			t.Fatalf("precondition: raise %d refused: %s", i+1, e2eharness.SpellFailReasonName(res.FailReason))
		}
		time.Sleep(gcd)
	}
	var skeletons []uint64
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline) && len(skeletons) < n; time.Sleep(250 * time.Millisecond) {
		skeletons = ownedUnits(bot, creatureLesserSkeleton)
	}
	if len(skeletons) < n {
		t.Fatalf("precondition: %d of %d skeletons visible", len(skeletons), n)
	}
	return skeletons
}

// Main project issue #288: raising another skeleton while the Life Force is full fails with "Not enough mana".
// The refusal is the Life Force capacity design; the reason must no longer be a missing-power error.
//
//	go test -tags=e2e ./e2e/coa/summonspets -run RaiseWithFullLifeForce -count=1 -v
func TestNecromancer_RaiseWithFullLifeForce(t *testing.T) {
	const reasonNoPower, reasonAlreadyHaveSummon = 85, 7
	bot := necromancer(t, "NcRaise", 8)
	raiseSkeletons(t, bot, 2)
	power, maxPower := bot.PlayerPower()
	res, err := bot.TryCast(t, spellRaiseLesserSkeleton, 0, 10*time.Second)
	if err != nil {
		t.Fatalf("third raise: %v", err)
	}
	t.Logf("third raise with 2 skeletons (mana %d/%d): success %v, reason %d %s", power, maxPower, res.Success,
		res.FailReason, e2eharness.SpellFailReasonName(res.FailReason))
	switch {
	case res.Success:
		t.Errorf("E2E_FAIL: a third skeleton was raised with a full Life Force (#288)")
	case res.FailReason == reasonNoPower:
		t.Errorf("E2E_FAIL: full Life Force refused as missing power, shown as \"Not enough mana\" (#288)")
	case res.FailReason == reasonAlreadyHaveSummon:
		t.Logf("E2E_PASS: full Life Force refused as \"You already control a summoned creature\" (#288)")
	default:
		t.Errorf("E2E_FAIL: unexpected refusal reason %d (#288)", res.FailReason)
	}
}

// Main project issue #285: in Undead: Protect, Grave March then Undead: Pacify leaves the minions standing where
// they stopped instead of returning to the Necromancer.
//
//	go test -tags=e2e ./e2e/coa/summonspets -run PacifyReturnsMinions -count=1 -v
func TestNecromancer_PacifyReturnsMinions(t *testing.T) {
	bot := necromancer(t, "NcPac", 8)
	castLanded(t, bot, spellUndeadProtect, 0, 3)
	skeletons := raiseSkeletons(t, bot, 2)
	x, y, z, m := bot.Pos()
	bot.Teleport(t, x+25, y, z, m)
	boar := spawnTarget(t, bot, creatureMottledBoar, 6, 0)
	bot.GM(t, ".npc set react 0") // the boar stays where it is
	bot.Teleport(t, x, y, z, m)
	time.Sleep(settle)
	_ = bot.World.SetTarget(boar)
	left := false
	for attempt := 1; attempt <= 3 && !left; attempt++ {
		if res := castLanded(t, bot, spellGraveMarch, boar, 3); !res.Success {
			t.Fatalf("precondition: Grave March refused: %s", e2eharness.SpellFailReasonName(res.FailReason))
		}
		for deadline := time.Now().Add(6 * time.Second); time.Now().Before(deadline) && !left; time.Sleep(100 * time.Millisecond) {
			for _, s := range skeletons {
				if o := bot.World.GetObject(s); o != nil && e2eharness.Distance3D(o.PosX, o.PosY, o.PosZ, x, y, z) > 10 {
					left = true
				}
			}
		}
		if !left {
			for _, sk := range skeletons {
				o := bot.World.GetObject(sk)
				if o == nil {
					t.Logf("skeleton 0x%X not visible", sk)
					continue
				}
				hp, _ := bot.UnitHP(sk)
				t.Logf("skeleton 0x%X hp %d dist %.1f target 0x%X (boar 0x%X, boar dist %.1f)", sk, hp,
					e2eharness.Distance3D(o.PosX, o.PosY, o.PosZ, x, y, z), bot.UnitTarget(sk), boar, bot.DistFrom(func() (float32, float32, float32) {
						if b := bot.World.GetObject(boar); b != nil {
							return b.PosX, b.PosY, b.PosZ
						}
						return 0, 0, 0
					}()))
			}
			t.Logf("Grave March attempt %d: no skeleton moved 10 yd", attempt)
			time.Sleep(gcd)
		}
	}
	if !left {
		t.Fatalf("precondition: no skeleton moved 10 yd toward the Grave March target")
	}
	if res := castLanded(t, bot, spellUndeadPacify, 0, 3); !res.Success {
		t.Fatalf("precondition: Undead: Pacify refused: %s", e2eharness.SpellFailReasonName(res.FailReason))
	}
	time.Sleep(6 * time.Second)
	far := 0
	for _, s := range skeletons {
		o := bot.World.GetObject(s)
		if o == nil {
			continue
		}
		d := e2eharness.Distance3D(o.PosX, o.PosY, o.PosZ, x, y, z)
		t.Logf("skeleton 0x%X is %.1f yd from the Necromancer", s, d)
		if d > 8 {
			far++
		}
	}
	if far > 0 {
		t.Errorf("E2E_FAIL: %d skeleton(s) stayed away after Undead: Pacify (#285)", far)
		return
	}
	t.Logf("E2E_PASS: the skeletons came back after Undead: Pacify (#285)")
}

// Main project issue #1425: in Undead: Assault the minions attack nearby critters on their own.
//
//	go test -tags=e2e ./e2e/coa/summonspets -run AssaultCritters -count=1 -v
func TestNecromancer_AssaultIgnoresCritters(t *testing.T) {
	bot := necromancer(t, "NcCrit", 15)
	castLanded(t, bot, spellUndeadAssault, 0, 3)
	skeletons := raiseSkeletons(t, bot, 2)
	rabbit := spawnTarget(t, bot, creatureRabbit, 0, 0)
	bot.GM(t, ".npc set react 0")
	_ = bot.World.SetTarget(0)
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		for _, s := range skeletons {
			if bot.UnitTarget(s) == rabbit {
				t.Errorf("E2E_FAIL: skeleton 0x%X attacked a critter in Undead: Assault (#1425)", s)
				return
			}
		}
		if unitDead(bot, rabbit) {
			t.Errorf("E2E_FAIL: the critter died next to Assault minions (#1425)")
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Logf("E2E_PASS: Assault minions left the critter alone for 15 s (#1425)")
}
