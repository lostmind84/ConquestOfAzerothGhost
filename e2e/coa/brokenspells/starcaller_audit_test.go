//go:build e2e

package brokenspells_test

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/azerothcore/AzerothGhost/e2e/e2eharness"
)

const (
	spellLunarEclipseAbility uint32 = 800386 // CasterAuraSpell 704519, "4 Stacks of Lunar Phase"
	spellBrightMoon          uint32 = 801226 // SPELLMOD_MAX_AURA_STACKS +3 on Lunar Phase (cap 4 -> 8)
	auraLunarPhaseStacks     uint32 = 802985
	auraLunarPhaseMarker     uint32 = 704519 // 4-stack marker applied by the module
	creatureHostileGolem     uint32 = 36     // harness default hostile creature (faction 14)
	spellFanOfKnivesR1       uint32 = 680703 // applies one Scattered Stars
	spellLunarLance          uint32 = 801132 // trigger 572315 starts the Scattered Stars consume

	powerMana uint32 = 0 // smsgSpellEnergizeLog (0x0151) is declared in pyromancer_generated_test.go
)

// starcallerLevel80 creates a level 80 Starcaller with GM mode off and god on, as the scenario harness does.
func starcallerLevel80(t *testing.T, prefix string) *e2eharness.ScenarioBot {
	t.Helper()
	bot := newBot(t, prefix, e2eharness.RaceNightElf, classStarcaller, 80)
	bot.CombatReady(t)
	t.Cleanup(func() { eraseOwnCharacter(t, bot) })
	return bot
}

// eraseOwnCharacter logs the bot out and deletes its character with CMSG_CHAR_DELETE, so the throwaway
// character does not stay on the server. It registers before the session close, hence runs first.
func eraseOwnCharacter(t *testing.T, bot *e2eharness.ScenarioBot) {
	guid := bot.World.CharGUID()
	if err := bot.World.SendLogout(); err != nil {
		t.Logf("cleanup: logout: %v", err)
		return
	}
	if err := bot.World.WaitForLogout(30 * time.Second); err != nil {
		t.Logf("cleanup: no logout completion: %v", err)
		return
	}
	if err := bot.World.DeleteCharacter(guid); err != nil {
		t.Logf("cleanup: delete: %v", err)
		return
	}
	time.Sleep(time.Second) // the server handles CMSG_CHAR_DELETE asynchronously
	t.Logf("cleanup: character %#x deleted", guid)
}

// setLunarPhase leaves the bot with exactly n Lunar Phase stacks and the 4-stack marker.
func setLunarPhase(t *testing.T, bot *e2eharness.ScenarioBot, n int) {
	t.Helper()
	bot.GM(t, fmt.Sprintf(".unaura %d", auraLunarPhaseStacks))
	bot.WaitAuraGone(t, auraLunarPhaseStacks, 2*time.Second)
	for i := 0; i < 2*n && bot.AuraStacks(auraLunarPhaseStacks) < n; i++ {
		bot.ApplyAura(t, auraLunarPhaseStacks)
		time.Sleep(300 * time.Millisecond) // let the aura update reach the client before reading the count
	}
	if got := bot.AuraStacks(auraLunarPhaseStacks); got < n {
		t.Fatalf("precondition: could not reach %d Lunar Phase stacks, have %d", n, got)
	}
	if !bot.HasAura(auraLunarPhaseMarker) {
		bot.ApplyAura(t, auraLunarPhaseMarker)
	}
}

// Main project issue #4273: with Bright Moon (Lunar Phase caps at 8) Lunar Eclipse needed all 8 stacks and
// spent them all. Expected: it is castable at exactly 4 stacks (no CASTER_AURASTATE refusal) and spends 4.
// Server-side proof: apps/coa-gameplay-test scenario starcaller-lunar-eclipse-threshold.
//
//	go test -tags=e2e -p 1 ./e2e/coa/brokenspells -run LunarEclipseFourStacks -count=1 -v
func TestStarcaller_LunarEclipseFourStacksWithBrightMoon(t *testing.T) {
	bot := starcallerLevel80(t, "ScLe4")
	bot.Learn(t, spellLunarEclipseAbility)
	bot.Learn(t, spellBrightMoon)

	// Exactly four stacks: must be castable and must spend them.
	setLunarPhase(t, bot, 4)
	before := bot.AuraStacks(auraLunarPhaseStacks)
	res, err := bot.TryCast(t, spellLunarEclipseAbility, 0, castTimeout)
	if err != nil {
		t.Fatalf("Lunar Eclipse at 4 stacks: %v", err)
	}
	if !res.Success {
		t.Fatalf("E2E_FAIL: Lunar Eclipse refused at %d Lunar Phase stacks with Bright Moon: %s (#4273)",
			before, e2eharness.SpellFailReasonName(res.FailReason))
	}
	bot.WaitAura(t, spellLunarEclipseAbility, 2*time.Second)
	time.Sleep(settle)
	// One passive gain may land between the cast and the read, hence <= 1.
	if after := bot.AuraStacks(auraLunarPhaseStacks); after > 1 {
		t.Errorf("E2E_FAIL: %d Lunar Phase stacks after a 4-stack Eclipse, want at most 1 (#4273)", after)
	} else {
		t.Logf("E2E_PASS: Eclipse cast at 4 stacks (had %d), %d left", before, after)
	}

	// Eight stacks: only four are spent.
	bot.GM(t, fmt.Sprintf(".unaura %d", spellLunarEclipseAbility)) // the Eclipse buff is not client-cancellable
	bot.WaitAuraGone(t, spellLunarEclipseAbility, 2*time.Second)
	bot.GM(t, ".cooldown") // the bot is its own selection after ApplyAura
	time.Sleep(gcd)
	setLunarPhase(t, bot, 8)
	res, err = bot.TryCast(t, spellLunarEclipseAbility, 0, castTimeout)
	if err != nil {
		t.Fatalf("Lunar Eclipse at 8 stacks: %v", err)
	}
	if !res.Success {
		t.Fatalf("Lunar Eclipse refused at 8 stacks: %s", e2eharness.SpellFailReasonName(res.FailReason))
	}
	bot.WaitAura(t, spellLunarEclipseAbility, 2*time.Second)
	time.Sleep(settle)
	if after := bot.AuraStacks(auraLunarPhaseStacks); after < 4 {
		t.Errorf("E2E_FAIL: %d Lunar Phase stacks left after an 8-stack Eclipse, want at least 4 (#4273)", after)
	} else {
		t.Logf("E2E_PASS: 8-stack Eclipse left %d stacks", after)
	}
}

// manaGainLog collects the mana the bot gains from SMSG_SPELLENERGIZELOG (spell effects, not regeneration).
type manaGainLog struct {
	mu     sync.Mutex
	events []manaGainEvent
}

type manaGainEvent struct {
	spellID uint32
	amount  uint32
}

func watchManaEnergize(t *testing.T, bot *e2eharness.ScenarioBot) *manaGainLog {
	t.Helper()
	log := &manaGainLog{}
	self := bot.World.CharGUID()
	cancel := bot.World.AddPacketHook(func(opcode uint16, data []byte) {
		if opcode != smsgSpellEnergizeLog {
			return
		}
		r := bytes.NewReader(data)
		if readPackedGUID(r) != self {
			return
		}
		readPackedGUID(r) // caster
		var body struct {
			SpellID uint32
			Power   uint32
			Amount  uint32
		}
		if binary.Read(r, binary.LittleEndian, &body) != nil || body.Power != powerMana {
			return
		}
		log.mu.Lock()
		log.events = append(log.events, manaGainEvent{body.SpellID, body.Amount})
		log.mu.Unlock()
	})
	t.Cleanup(cancel)
	return log
}

func (l *manaGainLog) total() (sum uint32, events []manaGainEvent) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, e := range l.events {
		sum += e.amount
	}
	return sum, append([]manaGainEvent(nil), l.events...)
}

func (l *manaGainLog) reset() {
	l.mu.Lock()
	l.events = nil
	l.mu.Unlock()
}

// consumeStar applies one Scattered Stars stack with Fan of Knives on target (retrying misses), then casts
// Lunar Lance until the consume returned mana or the attempts run out. It returns the mana energized.
func consumeStar(t *testing.T, bot *e2eharness.ScenarioBot, mana *manaGainLog, target uint64, lanceAttempts int) uint32 {
	t.Helper()
	for i := 0; i < 5 && bot.UnitAuraStacks(target, auraScatteredStars) == 0; i++ {
		castLanded(t, bot, spellFanOfKnivesR1, target, 2)
		time.Sleep(settle)
	}
	if bot.UnitAuraStacks(target, auraScatteredStars) == 0 {
		t.Fatalf("precondition: Fan of Knives never applied Scattered Stars")
	}
	mana.reset()
	for i := 0; i < lanceAttempts; i++ {
		castLanded(t, bot, spellLunarLance, target, 3)
		time.Sleep(4 * time.Second) // the consumer ticks every 500 ms
		if sum, _ := mana.total(); sum > 0 {
			return sum
		}
	}
	return 0
}

// Main project issue #4224: a Scattered Stars consumer that kills its target left the stars unconsumed and
// returned no mana. Expected: the Lance that kills a target holding a star still triggers the consume
// (SMSG_SPELLENERGIZELOG to the bot), as it does on a living target (the control, which makes a red result
// interpretable). Server-side proof: scenario starcaller-scattered-stars-corpse (mana ratio >= 0.8).
//
//	go test -tags=e2e -p 1 ./e2e/coa/brokenspells -run ScatteredStarsCorpse -count=1 -v
func TestStarcaller_ScatteredStarsConsumeOnCorpse(t *testing.T) {
	bot := starcallerLevel80(t, "ScCrp")
	bot.Learn(t, spellFanOfKnivesR1)
	bot.Learn(t, spellLunarLance)
	mana := watchManaEnergize(t, bot)

	alive := spawnTarget(t, bot, creatureHostileGolem, 80)
	control := consumeStar(t, bot, mana, alive, 2)
	if control == 0 {
		t.Fatalf("precondition: consume on a living target returned no mana (energize events: none), test not interpretable")
	}
	t.Logf("control: consume on a living target energized %d mana", control)
	bot.Damage(t, alive, 1_000_000) // clear the first target before the second

	dying := spawnTarget(t, bot, creatureHostileGolem, 80)
	// A Lance can miss and leave the target alive (the consume still runs), so retry whole rounds: cooldowns
	// reset, star present, health cut to 1, Lance, and only a round that ends with a corpse counts.
	var got uint32
	dead := false
	for round := 1; round <= 4 && !dead; round++ {
		_ = bot.World.SetTarget(bot.World.CharGUID()) // .cooldown applies to the selection
		bot.GM(t, ".cooldown")
		_ = bot.World.SetTarget(dying)
		for i := 0; i < 5 && bot.UnitAuraStacks(dying, auraScatteredStars) == 0; i++ {
			castLanded(t, bot, spellFanOfKnivesR1, dying, 2)
			time.Sleep(settle)
		}
		if bot.UnitAuraStacks(dying, auraScatteredStars) == 0 {
			t.Fatalf("precondition: Fan of Knives never applied Scattered Stars to the second target")
		}
		if hp, _ := bot.UnitHP(dying); hp > 1 {
			bot.Damage(t, dying, hp-1)
		}
		mana.reset()
		castLanded(t, bot, spellLunarLance, dying, 3)
		time.Sleep(4 * time.Second)
		got, _ = mana.total()
		obj := bot.World.GetObject(dying)
		dead = obj != nil && !obj.IsAlive()
		_, events := mana.total()
		t.Logf("corpse round %d: target dead=%v, energize events %+v", round, dead, events)
	}
	if !dead {
		t.Fatalf("precondition: no Lance killed the target in 4 rounds")
	}
	if got == 0 {
		t.Errorf("E2E_FAIL: consume on a corpse returned no mana, living target returned %d (#4224)", control)
		return
	}
	t.Logf("E2E_PASS: consume on a corpse energized %d mana (living target %d)", got, control)
}

const (
	smsgMonsterMoveOpcode uint16 = 0x00DD
	splineFlagParabolic   uint32 = 0x00000800
	splineFlagAnimation   uint32 = 0x00400000
)

// jumpDestination reads the destination of a parabolic SMSG_MONSTER_MOVE after the mover GUID. The harness
// position cache does not follow parabolic splines (it parses their destination as 0,0), so the knockback end
// point is read from the packet, as e2e/coa/talents does for Burrow Bolt.
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

type knockJump struct {
	at   time.Time
	dest [3]float32
}

// Main project issue #4013: Stellar Drift slows nearby enemies, then knocks them back after 3 seconds. The
// reporter says the knockback never happens. Expected: the creature carries the slow 802773, is not moved
// while the slow runs, and ends up at least 15 yards further away (the native arc is about 25 yd).
// Server-side proof: scenario starcaller-stellar-drift-knockback.
//
//	go test -tags=e2e -p 1 ./e2e/coa/brokenspells -run StellarDriftKnockback -count=1 -v
func TestStarcaller_StellarDriftKnocksBackAfterSlow(t *testing.T) {
	const (
		spellStellarDrift uint32 = 800501
		auraStellarSlow   uint32 = 802773
	)
	bot := starcallerLevel80(t, "ScDrft")
	bot.Learn(t, spellStellarDrift)
	// Hostile (faction 14) creature: training dummies are neutral and are not Stellar Drift targets.
	golem := spawnTarget(t, bot, creatureHostileGolem, 80)
	time.Sleep(settle)
	obj := bot.World.GetObject(golem)
	if obj == nil || !obj.HasKnownPosition() {
		t.Fatalf("precondition: no position for the creature")
	}
	bx, by, _, _ := bot.Pos()
	dist := func(x, y float32) float32 { return e2eharness.Distance3D(bx, by, 0, x, y, 0) }
	before := dist(obj.PosX, obj.PosY)
	t.Logf("creature %.1f yd away before the cast", before)

	var mu sync.Mutex
	var jumps []knockJump
	cancelMoves := bot.World.AddPacketHook(func(op uint16, data []byte) {
		if op != smsgMonsterMoveOpcode {
			return
		}
		r := bytes.NewReader(data)
		if readPackedGUID(r) != golem {
			return
		}
		if dest, ok := jumpDestination(r); ok {
			mu.Lock()
			jumps = append(jumps, knockJump{time.Now(), dest})
			mu.Unlock()
		}
	})
	defer cancelMoves()

	var res e2eharness.SpellCastResult
	var slowSeen time.Time
	for attempt := 1; attempt <= 3; attempt++ {
		res = castLanded(t, bot, spellStellarDrift, 0, 2)
		if !res.Success {
			t.Fatalf("Stellar Drift refused: %s", e2eharness.SpellFailReasonName(res.FailReason))
		}
		start := time.Now()
		for time.Since(start) < 1500*time.Millisecond && slowSeen.IsZero() {
			if bot.UnitHasAura(golem, auraStellarSlow) {
				slowSeen = time.Now()
			}
			time.Sleep(100 * time.Millisecond)
		}
		if !slowSeen.IsZero() {
			break
		}
		t.Logf("attempt %d: no slow on the creature (miss?)", attempt)
		time.Sleep(6 * time.Second)
		_ = bot.World.SetTarget(bot.World.CharGUID()) // .cooldown applies to the selection
		bot.GM(t, ".cooldown")
	}
	if slowSeen.IsZero() {
		t.Fatalf("precondition: Stellar Drift never slowed the creature")
	}

	// The knockback comes 3 s after the slow; wait for the jump packet, then let the creature settle.
	deadline := time.Now().Add(7 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		n := len(jumps)
		mu.Unlock()
		if n > 0 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	mu.Lock()
	got := append([]knockJump(nil), jumps...)
	mu.Unlock()
	if len(got) == 0 {
		t.Errorf("E2E_FAIL: no knockback spline sent for the slowed creature within 7 s (#4013)")
		return
	}
	first := got[0]
	after := dist(first.dest[0], first.dest[1])
	t.Logf("E2E_MEASURE: slow seen, knockback spline %.1f s later, %.1f yd -> %.1f yd from the caster",
		first.at.Sub(slowSeen).Seconds(), before, after)
	if after-before < 15 {
		t.Errorf("E2E_FAIL: knockback moves the creature %.1f yd away, want at least 15 (#4013)", after-before)
		return
	}
	t.Logf("E2E_PASS: Stellar Drift knocked the creature back %.1f yd", after-before)
}
