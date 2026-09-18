//go:build e2e

package resources_test

import (
	"testing"
	"time"

	"github.com/azerothcore/AzerothGhost/e2e/e2eharness"
)

const (
	spellCursedForm      uint32 = 562572 // Blood Curse, rank "Cursed Form"; grants both Cursed Form markers
	spellRotclawRank1    uint32 = 804197 // the rank a level 17 Bloodmage owns (rank 2 is learned at 18)
	spellRavenousStrike2 uint32 = 501671 // control: carries effect 142 -> Ravenous Strike (Energize) 805352
	spellBloodRush       uint32 = 560254 // passive: periodic damage has a 20% chance to grant Blood Rush
	auraBloodRush        uint32 = 504551 // the haste buff Blood Rush grants

	rotclawLevel = 17 // level reported in #3948
	dummyLevel   = 15
)

// Main project issue #3948: Rotclaw generates no Rage.
//
// Rotclaw's own description in Spell.dbc promises it ("Ravage all nearby enemies, dealing ... Shadow
// damage, generating Rage and infecting their wounds ..."), but none of its three effects is an
// energize and no companion "Rotclaw (Energize)" record exists, unlike every other Rage-generating
// Bloodmage ability (Ravenous Strike, Bloodmoon Blast, Sanguine Rupture and Lunge all carry effect
// 142 pointing at their own Energize spell). Unit::DealDamage only pays Rage for weapon swings, so a
// spell effect grants none by itself.
//
// Ravenous Strike is the control: same class, same form, same melee context, and it does carry the
// energize effect, so it proves the measurement sees Rage when the data asks for it.
//
//	go test -tags=e2e ./e2e/coa/resources -run Rotclaw -count=1 -v
func TestBloodmage_RotclawGeneratesRage(t *testing.T) {
	bot := newBot(t, "BmRot", e2eharness.RaceBloodElf, classBloodmage, rotclawLevel)
	bot.Learn(t, spellCursedForm)
	bot.Learn(t, spellRotclawRank1)
	bot.Learn(t, spellRavenousStrike2)
	dummy := spawnTarget(t, bot, e2eharness.CreatureHeroicTrainingDummy, dummyLevel)
	bot.CombatReady(t) // no `.cheat power`: it pins Rage at maximum and hides any gain

	castLanded(t, bot, spellCursedForm, 0, 2)
	bot.WaitAura(t, spellCursedForm, 3*time.Second)
	time.Sleep(gcd)

	for _, c := range []struct {
		name    string
		spellID uint32
		issue   int
	}{
		{"RavenousStrikeControl", spellRavenousStrike2, 0},
		{"Rotclaw", spellRotclawRank1, 3948},
	} {
		t.Run(c.name, func(t *testing.T) {
			bot.Face(t, dummy)
			before := rage(bot)
			if res := castLanded(t, bot, c.spellID, dummy, 4); !res.Success {
				t.Fatalf("%d never landed: %s", c.spellID, e2eharness.SpellFailReasonName(res.FailReason))
			}
			after := peakRage(bot, 2*time.Second)
			t.Logf("spell %d: Rage %d -> %d (internal tenths)", c.spellID, before, after)
			if after <= before {
				if c.issue == 0 {
					t.Fatalf("control spell %d granted no Rage; the measurement is not trustworthy", c.spellID)
				}
				t.Errorf("E2E_FAIL: Rotclaw granted no Rage (%d -> %d) although its description promises it (#%d)",
					before, after, c.issue)
				return
			}
			t.Logf("E2E_PASS: spell %d raised Rage by %d (internal tenths)", c.spellID, after-before)
			time.Sleep(gcd)
		})
	}
}

// Main project issue #3981: Blood Rush never procs off Rotclaw's periodic damage.
//
// Blood Rush 560254 is a passive whose effect 0 is aura 42 (proc trigger spell) on 504551, with
// ProcChance 20 and a description that names periodic damage as the trigger - but its ProcFlags
// field is 0 and no spell_proc row backs it, so the proc system has nothing to match and the aura
// can never fire.
//
//	go test -tags=e2e ./e2e/coa/resources -run BloodRush -count=1 -v
func TestBloodmage_BloodRushProcsOnPeriodicDamage(t *testing.T) {
	bot := newBot(t, "BmRush", e2eharness.RaceBloodElf, classBloodmage, rotclawLevel)
	bot.Learn(t, spellCursedForm)
	bot.Learn(t, spellRotclawRank1)
	bot.Learn(t, spellBloodRush)
	dummy := spawnTarget(t, bot, e2eharness.CreatureHeroicTrainingDummy, dummyLevel)
	bot.CombatReady(t) // no `.cheat power`: it pins Rage at maximum and hides any gain

	castLanded(t, bot, spellCursedForm, 0, 2)
	bot.WaitAura(t, spellCursedForm, 3*time.Second)
	time.Sleep(gcd)

	// 20% per tick, one tick a second: a bleed kept up for 30 s is over 99.8% likely to proc at
	// least once if the proc is wired at all.
	deadline := time.Now().Add(30 * time.Second)
	casts := 0
	for time.Now().Before(deadline) {
		bot.Face(t, dummy)
		if res := castLanded(t, bot, spellRotclawRank1, dummy, 3); res.Success {
			casts++
		}
		for wait := time.Now().Add(8 * time.Second); time.Now().Before(wait); {
			if bot.HasAura(auraBloodRush) {
				t.Logf("E2E_PASS: Blood Rush %d granted after %d Rotclaw cast(s)", auraBloodRush, casts)
				return
			}
			time.Sleep(200 * time.Millisecond)
		}
	}
	t.Errorf("E2E_FAIL: Blood Rush %d never granted over %d Rotclaw cast(s) of bleed ticks (#3981)",
		auraBloodRush, casts)
}
