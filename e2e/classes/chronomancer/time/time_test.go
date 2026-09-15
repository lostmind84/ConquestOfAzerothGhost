//go:build e2e

package time_test

import (
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"

	"github.com/azerothcore/AzerothGhost/e2e/classes/chronomancer"
	"github.com/azerothcore/AzerothGhost/e2e/e2eharness"
)

// Ability & Aura constants for the Time specialization
const (
	AbilityAeonofRenewal    uint32 = 806290 // Aeon of Renewal
	AbilityAeonofResilience uint32 = 806291 // Aeon of Resilience
	AbilityAeonofProtection uint32 = 806292 // Aeon of Protection
	AbilityAeonofOblivion   uint32 = 806293 // Aeon of Oblivion

	AbilityEpoch        uint32 = 504575 // Epoch (Rank 8)
	AbilityReverseWound uint32 = 572628 // Reverse Wound (Rank 9)
	AbilityAcceleratedRecoveryRank7 uint32 = 501777 // Accelerated Recovery (Rank 7)

	// Chronomancer
	TalentAcceleratedRecovery uint32 = 30277 // Talent: Accelerated Recovery

	//Time
	TalentRipple       uint32 = 31182 // Talent: Ripple
	TalentEndlessSands uint32 = 6700  // Talent: Endless Sands

	AuraSandsOfTime            uint32 = 804488 // Sands of Time Aura ID (reduces Epoch cast time)
	AuraEndlessSands           uint32 = 806728 // Endless Sands Aura ID (reduces Reverse Wound cast time)
	AuraAcceleratedRecoveryHoT uint32 = 804500 // Accelerated Recovery HoT Aura ID
)

func TestChronomancer_TimeTalentsAndAbilities(t *testing.T) {
	t.Parallel()

	// Logs in and becomes a Level 60 Human Chronomancer
	bot := e2eharness.NewSolo(t, e2eharness.ScenarioOpts{
		Prefix: "Chrono",
		Race:   e2eharness.RaceHuman,
		Class:  chronomancer.ClassID,
		Level:  60,
	})

	pad := e2eharness.PackagePad(t)
	bot.TeleportPad(t, pad)
	bot.CheatPower(t)

	// Changes the Specialization to "Time" and confirms the switch via KnowsAbility
	bot.SetSpecialization(t, chronomancer.SpecTime, chronomancer.AbilityTimeBaseline)
	t.Logf("E2E_PASS: bot learned Time baseline Ability %s", e2eharness.DescribeSpell(chronomancer.AbilityTimeBaseline))

	// ==========================================
	// CoA Hidden Auras (Class + Spec Passives)
	// ==========================================
	// These passive auras should be applied automatically by the server when a
	// Chronomancer selects the Time specialization. They modify balance values
	// (e.g. healing coefficients). Currently bugged: server does not apply them.
	time.Sleep(500 * time.Millisecond) // allow aura sync after spec change

	coaAuras := []struct {
		id   uint32
		name string
	}{
		{chronomancer.AuraChronomancerClass, "CoA Aura - Chronomancer Class"},
		{chronomancer.AuraChronomancerTime, "CoA Aura - Chronomancer Time"},
	}

	for _, aura := range coaAuras {
		if bot.HasAura(aura.id) {
			t.Logf("E2E_PASS: hidden aura '%s' (%d) is active", aura.name, aura.id)
		} else {
			t.Logf("BUG DETECTED: hidden aura '%s' (%d) is NOT applied — server should auto-apply on spec change",
				aura.name, aura.id)
		}
	}

	// ==========================================
	// Aeons (Stances)
	// ==========================================
	// At Level 60 in Time spec, the player should know all 4 Aeons
	aeons := []uint32{
		AbilityAeonofRenewal,
		AbilityAeonofResilience,
		AbilityAeonofProtection,
		AbilityAeonofOblivion,
	}

	for _, aeon := range aeons {
		if bot.World.KnowsSpell(aeon) {
			t.Logf("E2E_PASS: bot knows Aeon '%s'", e2eharness.DescribeSpell(aeon))
		} else {
			t.Errorf("E2E_FAIL: bot does not know Aeon '%s'", e2eharness.DescribeSpell(aeon))
		}

		// Detect if any Aeon was sent/learned more than once (e.g. server bug where Aeon of Resilience is learned twice)
		count := bot.SpellLearnedCount(aeon)
		if count > 1 {
			t.Errorf("E2E_FAIL: Aeon '%s' (%d) was learned %d times (duplicate learn packets from server)",
				e2eharness.DescribeSpell(aeon), aeon, count)
		}
	}
	// Aeons are "Stances" and should be mutually exclusive
	t.Log("Aeons are 'stances' and should be mutually exclusive")
	for _, currentAeon := range aeons {
		t.Logf("Casting Aeon '%s'...", e2eharness.DescribeSpell(currentAeon))
		bot.CastOrGM(t, currentAeon, 0, 5*time.Second)
		time.Sleep(500 * time.Millisecond) // Wait for aura sync

		t.Logf("Active auras after casting %s: %v", e2eharness.DescribeSpell(currentAeon), bot.World.SelfAuras())

		// Verify current Aeon is active
		if !bot.HasAura(currentAeon) {
			t.Errorf("E2E_FAIL: expected active aura '%s' after cast, but aura is missing", e2eharness.DescribeSpell(currentAeon))
		} else {
			t.Logf("E2E_PASS: '%s' is active", e2eharness.DescribeSpell(currentAeon))
		}

		// Verify all other Aeons are NOT active
		for _, otherAeon := range aeons {
			if otherAeon != currentAeon {
				if bot.HasAura(otherAeon) {
					t.Errorf("E2E_FAIL: mutually exclusive aura '%s' is unexpectedly active while in '%s'",
						e2eharness.DescribeSpell(otherAeon), e2eharness.DescribeSpell(currentAeon))
				} else {
					t.Logf("E2E_PASS: '%s' is inactive as expected", e2eharness.DescribeSpell(otherAeon))
				}
			}
		}

		// Aeons have a 5s cooldown; wait 6s before casting the next Aeon to account for cooldown and network lag
		time.Sleep(6 * time.Second)
	}



	// ==========================================
	// Epoch + Sands of Time Proc ==> Increase Epoch Cast Speed
	// ==========================================
	// At Level 60, the player should know Ranks 1-8 of Epoch
	epochSpellIDs := []uint32{
		801270, // Epoch (Rank 1)
		501779, // Epoch (Rank 2)
		501780, // Epoch (Rank 3)
		501781, // Epoch (Rank 4)
		501782, // Epoch (Rank 5)
		501783, // Epoch (Rank 6)
		501784, // Epoch (Rank 7)
		504575, // Epoch (Rank 8)
	}
	for _, spellid := range epochSpellIDs {
		if bot.World.KnowsSpell(spellid) {
			t.Logf("E2E_PASS: bot knows spell '%s'", e2eharness.DescribeSpell(spellid))
		} else {
			t.Errorf("E2E_FAIL: bot does not know spell '%s'", e2eharness.DescribeSpell(spellid))
		}
	}

	// Casting [Epoch] grants a stack of [Sands of Time] which makes Epoch cast 20% faster
	// At 5 stacks of [Sands of Time], [Epoch] will be Instant Cast and the stacks of [Sands of Time] will be reset
	// Progression of expected cast durations:
	// Cast 1 (0 stacks): 2000ms (2.0s) -> grants stack 1
	// Cast 2 (1 stack):  1600ms (1.6s) -> grants stack 2
	// Cast 3 (2 stacks): 1200ms (1.2s) -> grants stack 3
	// Cast 4 (3 stacks): 800ms  (0.8s) -> grants stack 4
	// Cast 5 (4 stacks): 400ms  (0.4s) -> grants stack 5
	// Cast 6 (5 stacks): 0ms    (Instant Cast) -> consumes all 5 stacks (0 stacks remaining, no aura)
	// Cast 7 (0 stacks): resets back to 2000ms (2.0s)
	steps := []struct {
		castNum           int
		expectedCastBarMs uint32
		expectedDur       time.Duration
		tolerance         time.Duration
		isInstant         bool
		expectedAfter     int // stacks expected after cast
	}{
		{1, 2000, 2000 * time.Millisecond, 300 * time.Millisecond, false, 1},
		{2, 1600, 1600 * time.Millisecond, 300 * time.Millisecond, false, 2},
		{3, 1200, 1200 * time.Millisecond, 300 * time.Millisecond, false, 3},
		{4, 800, 800 * time.Millisecond, 250 * time.Millisecond, false, 4},
		{5, 400, 400 * time.Millisecond, 200 * time.Millisecond, false, 5},
		{6, 0, 0 * time.Millisecond, 500 * time.Millisecond, true, 0},
	}

	for _, s := range steps {
		time.Sleep(500 * time.Millisecond) // Delay between casts for GCD, network latency, and aura stack sync

		res, dur := bot.CastAndMeasure(t, AbilityEpoch, 0, 10*time.Second)
		if !res.Success {
			t.Fatalf("Cast %d failed: reason=%d (%s)", s.castNum, res.FailReason, e2eharness.SpellFailReasonName(res.FailReason))
		}

		// 1. Verify the exact Cast Bar duration reported by the server in SMSG_SPELL_START
		if s.isInstant {
			if res.CastTimeMs != 0 {
				t.Errorf("E2E_FAIL: Cast %d expected Cast Bar 0ms (Instant Cast), got %dms", s.castNum, res.CastTimeMs)
			} else {
				t.Logf("E2E_PASS: Cast %d Cast Bar duration: 0ms (Instant Cast)", s.castNum)
			}
		} else {
			diff := int(res.CastTimeMs) - int(s.expectedCastBarMs)
			if diff < -5 || diff > 5 {
				t.Errorf("E2E_FAIL: Cast %d expected Cast Bar ~%dms, got %dms", s.castNum, s.expectedCastBarMs, res.CastTimeMs)
			} else {
				t.Logf("E2E_PASS: Cast %d Cast Bar duration: %dms (expected ~%dms)", s.castNum, res.CastTimeMs, s.expectedCastBarMs)
			}
		}

		// 2. Verify the client-measured wall clock cast duration
		if s.isInstant {
			if dur > s.tolerance {
				t.Errorf("E2E_FAIL: Cast %d expected Instant cast (<%v), took %v", s.castNum, s.tolerance, dur)
			} else {
				t.Logf("E2E_PASS: Cast %d measured wall-clock duration: %v (round-trip)", s.castNum, dur)
			}
		} else {
			minDur := s.expectedDur - s.tolerance
			maxDur := s.expectedDur + s.tolerance
			if dur < minDur || dur > maxDur {
				t.Errorf("E2E_FAIL: Cast %d measured duration %v outside expected [%v, %v] (target %v)", s.castNum, dur, minDur, maxDur, s.expectedDur)
			} else {
				t.Logf("E2E_PASS: Cast %d measured duration %v within [%v, %v] (target %v)", s.castNum, dur, minDur, maxDur, s.expectedDur)
			}
		}

		// 3. Verify aura stack count after cast
		stacks := bot.AuraStacks(AuraSandsOfTime)
		if stacks != s.expectedAfter {
			t.Errorf("E2E_FAIL: after Cast %d, expected %d stacks, got %d", s.castNum, s.expectedAfter, stacks)
		} else {
			t.Logf("E2E_PASS: after Cast %d, player has %d stacks of Sands of Time", s.castNum, stacks)
		}
	}

	// Verify that after Cast 6, the 5 stacks were consumed and no Sands of Time aura remains
	bot.AssertNoAura(t, AuraSandsOfTime)
	t.Log("E2E_PASS: 5 stacks consumed on 6th cast; no Sands of Time aura remains")

	// Cast 7: Verify it resets back to 2.0s (2000ms) after stacks are consumed
	t.Log("Verifying Epoch cast time resets back to 2.0s after stacks were consumed...")
	time.Sleep(1500 * time.Millisecond) // Allow GCD from the instant cast to clear
	res, dur := bot.CastAndMeasure(t, AbilityEpoch, 0, 10*time.Second)
	if !res.Success {
		t.Fatalf("Cast 7 failed: reason=%d (%s)", res.FailReason, e2eharness.SpellFailReasonName(res.FailReason))
	}
	diff := int(res.CastTimeMs) - 2000
	if diff < -5 || diff > 5 {
		t.Errorf("E2E_FAIL: Cast 7 expected Cast Bar 2000ms, got %dms", res.CastTimeMs)
	} else {
		t.Logf("E2E_PASS: Cast 7 Cast Bar confirmed reset to %dms", res.CastTimeMs)
	}
	if dur < 1700*time.Millisecond || dur > 2300*time.Millisecond {
		t.Errorf("E2E_FAIL: Cast 7 measured duration %v outside expected [1.7s, 2.3s] (target 2.0s)", dur)
	} else {
		t.Logf("E2E_PASS: Cast 7 measured duration %v confirmed reset back to ~2.0s", dur)
	}

	// --- Final Summary ---
	if t.Failed() {
		t.Fatal("E2E_SUMMARY: Epoch Sands of Time cast progression test failed!")
	}
	t.Log("E2E_SUMMARY: Epoch Sands of Time cast progression (2.0s -> 1.6s -> 1.2s -> 0.8s -> 0.4s -> Instant -> reset to 2.0s) passed!")




	// ==========================================
	// Epoch + Endless Sands Talent ==> Increase Reverse Wound Cast Speed
	// ==========================================
	t.Log("Learning Endless Sands (6700) talent...")
	bot.SetTalentRank(t, TalentEndlessSands, 1)
	time.Sleep(1 * time.Second)

	// 4. Cast Epoch 5 times (with 2.5s between casts) and verify that we have 5 stacks of Endless Sands
	t.Log("Casting Epoch 5 times with 2.5s interval to build 5 stacks of Endless Sands...")
	for i := 1; i <= 5; i++ {
		bot.CastOrGM(t, AbilityEpoch, 0, 5*time.Second)
		time.Sleep(2500 * time.Millisecond)

		stacks := bot.AuraStacks(AuraEndlessSands)
		if stacks != i {
			t.Errorf("E2E_FAIL: after Epoch cast %d, expected %d stacks of Endless Sands, got %d", i, i, stacks)
		} else {
			t.Logf("E2E_PASS: after Epoch cast %d, player has %d stacks of Endless Sands", i, stacks)
		}
	}

	bot.AssertHasAura(t, AuraEndlessSands)
	if stacks := bot.AuraStacks(AuraEndlessSands); stacks != 5 {
		t.Fatalf("E2E_FAIL: expected 5 stacks of Endless Sands after 5 Epoch casts, got %d", stacks)
	}

	// 5. Cast Reverse Wound and verify that it was Instant Cast (due to 5x 20% = 100% reduction from Endless Sands)
	t.Log("Casting Reverse Wound and verifying it is Instant Cast...")
	rwRes, rwDur := bot.CastAndMeasure(t, AbilityReverseWound, 0, 5*time.Second)
	if !rwRes.Success {
		t.Fatalf("Reverse Wound cast failed: reason=%d (%s)", rwRes.FailReason, e2eharness.SpellFailReasonName(rwRes.FailReason))
	}

	// Verify server Cast Bar duration is 0ms (Instant Cast)
	if rwRes.CastTimeMs != 0 {
		t.Errorf("E2E_FAIL: Reverse Wound expected Cast Bar 0ms (Instant Cast), got %dms", rwRes.CastTimeMs)
	} else {
		t.Logf("E2E_PASS: Reverse Wound Cast Bar confirmed 0ms (Instant Cast)")
	}

	// Verify measured wall-clock round-trip duration is instant (< 500ms)
	if rwDur > 500*time.Millisecond {
		t.Errorf("E2E_FAIL: Reverse Wound expected instant duration (<500ms), took %v", rwDur)
	} else {
		t.Logf("E2E_PASS: Reverse Wound measured duration %v (Instant Cast)", rwDur)
	}

	// Verify Endless Sands stacks were consumed
	t.Log("Verifying that all Endless Sands stacks have been consumed...")
	bot.AssertNoAura(t, AuraEndlessSands)
	t.Log("E2E_PASS: Endless Sands 5 stacks were consumed after casting Reverse Wound; no aura remains")




	// ==========================================
	// Accelerated Recovery
	// ==========================================
	bot.SetTalentRank(t, TalentAcceleratedRecovery, 1)

	// At Level 60, the player should know Ranks 1-7 of Accelerated Recovery
	acceleratedrecoverySpellIDs := []uint32{
		800857, // Accelerated Recovery (Rank 1)
		501772, // Accelerated Recovery (Rank 2)
		501773, // Accelerated Recovery (Rank 3)
		501774, // Accelerated Recovery (Rank 4)
		501775, // Accelerated Recovery (Rank 5)
		501776, // Accelerated Recovery (Rank 6)
		501777, // Accelerated Recovery (Rank 7)
	}
	for _, spellid := range acceleratedrecoverySpellIDs {
		if bot.World.KnowsSpell(spellid) {
			t.Logf("E2E_PASS: bot knows spell '%s'", e2eharness.DescribeSpell(spellid))
		} else {
			t.Errorf("E2E_FAIL: bot does not know spell '%s'", e2eharness.DescribeSpell(spellid))
		}
	}





	// Verify casting Accelerated Recovery Rank 7 on self results in HoT Aura (ID 804500) with ~15s duration
	t.Log("Casting Accelerated Recovery Rank 7 on self...")
	castRes := bot.Cast(t, AbilityAcceleratedRecoveryRank7, 0, 5*time.Second)
	if !castRes.Success {
		t.Fatalf("Accelerated Recovery Rank 7 cast failed: reason=%d (%s)",
			castRes.FailReason, e2eharness.SpellFailReasonName(castRes.FailReason))
	}

	bot.WaitAura(t, AuraAcceleratedRecoveryHoT, 3*time.Second)
	bot.AssertHasAura(t, AuraAcceleratedRecoveryHoT)

	hotDur := bot.AuraDuration(AuraAcceleratedRecoveryHoT)
	if hotDur <= 14*time.Second || hotDur >= 16*time.Second {
		t.Errorf("E2E_FAIL: Accelerated Recovery HoT aura expected duration ~15s (>14s and <16s), got %v", hotDur)
	} else {
		t.Logf("E2E_PASS: Accelerated Recovery HoT aura applied with duration %v (>14s and <16s)", hotDur)
	}

	//706777 = Eternal talent ID
	//TODO: check that the +6s duration to Accelerated Recovery has been applied and HoT now lasts ~21s.
}


