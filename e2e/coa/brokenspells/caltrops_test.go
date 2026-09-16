//go:build e2e

package brokenspells_test

import (
	"bytes"
	"encoding/binary"
	"sync/atomic"
	"testing"
	"time"

	"github.com/azerothcore/AzerothGhost/client"
	"github.com/azerothcore/AzerothGhost/e2e/e2eharness"
)

const (
	spellCaltrops       uint32 = 503679 // E1 summons creature 506010, BasePoints 11, radius 8
	spellCaltropsDamage uint32 = 504447
	spellVault          uint32 = 500085
	spellRenegade       uint32 = 524812 // passive: Vault drops caltrops (525054)
	creatureCaltrop     uint32 = 506010
	spellRenegadeDrop   uint32 = 525054 // cast by the module on Vault when the caster has Renegade
	talentRenegade      uint32 = 7093   // CoA talent entry of Renegade 524812
	witchHunterLevel           = 46     // level in #917 and #901
)

// Main project issues #917 and #257: Caltrops creates one enormous caltrop that damages every second
// and is never used up; the reports expect several small caltrops that are consumed when they hit.
// The "used up" expectation comes from the reports, not from a server-side record.
//
//	go test -tags=e2e ./e2e/coa/brokenspells -run Caltrops -count=1 -v
func TestWitchHunter_CaltropsSpreadAndAreConsumed(t *testing.T) {
	bot := newBot(t, "WhCal", e2eharness.RaceOrc, classWitchHunter, witchHunterLevel)
	bot.Learn(t, spellCaltrops)
	dummy := spawnTarget(t, bot, e2eharness.CreatureHeroicTrainingDummy, witchHunterLevel-2)
	bot.CombatReadyFull(t)
	log := watchDamage(t, bot, spellCaltropsDamage)

	x, y, z, _ := bot.Pos()
	if res := bot.CastAtPosition(t, spellCaltrops, x, y, z, castTimeout); !res.Success {
		t.Fatalf("Caltrops refused: %s", e2eharness.SpellFailReasonName(res.FailReason))
	}
	// Caltrops next to the dummy can be used up within 250 ms: record every caltrop seen during the first second.
	seen := map[uint64]struct{}{}
	for deadline := time.Now().Add(settle); time.Now().Before(deadline); time.Sleep(50 * time.Millisecond) {
		for _, u := range bot.UnitsByEntry(40, creatureCaltrop) {
			seen[u.GUID] = struct{}{}
		}
	}
	caltrops := make([]uint64, 0, len(seen))
	for guid := range seen {
		caltrops = append(caltrops, guid)
	}
	t.Logf("caltrop units after one cast: %d", len(caltrops))
	if len(caltrops) == 0 {
		t.Fatalf("precondition: no creature %d appeared", creatureCaltrop)
	}

	time.Sleep(5 * time.Second)
	hits := 0
	for _, ev := range log.snapshot() {
		if ev.target == dummy {
			hits++
		}
	}
	left := len(bot.UnitsByEntry(40, creatureCaltrop))
	t.Logf("after 5 s on the caltrops: %d damage logs on the dummy, %d caltrop units left", hits, left)

	switch {
	case len(caltrops) < 2 && hits < 2:
		t.Errorf("E2E_FAIL: Caltrops spawned %d caltrop unit(s) and hit %d time(s), want several small ones (#917)",
			len(caltrops), hits)
	case hits > 0 && left == len(caltrops):
		t.Errorf("E2E_FAIL: the dummy took %d caltrop hits and no caltrop was used up (#917)", hits)
	case hits == 0:
		t.Errorf("E2E_FAIL: a dummy standing on the caltrops took no damage (#917)")
	default:
		t.Logf("E2E_PASS: %d caltrops, %d hits, %d left", len(caltrops), hits, left)
	}
}

// Main project issue #901: with Renegade, Vault does not drop caltrops at the starting position.
// Reported before e980f4b59 (#217) made Vault displace the caster.
//
//	go test -tags=e2e ./e2e/coa/brokenspells -run RenegadeVault -count=1 -v
func TestWitchHunter_RenegadeVaultDropsCaltrops(t *testing.T) {
	bot := newBot(t, "WhRen", e2eharness.RaceOrc, classWitchHunter, witchHunterLevel)
	bot.Learn(t, spellVault)
	bot.SetTalentRank(t, talentRenegade, 1)
	bot.Learn(t, spellRenegade)
	bot.CombatReadyFull(t)
	time.Sleep(settle) // Renegade is a hidden passive (SPELL_ATTR0_DO_NOT_DISPLAY): never visible to the bot

	var dropCast atomic.Int32
	cancel := bot.World.AddPacketHook(func(opcode uint16, data []byte) {
		if opcode != client.SmsgSpellGo {
			return
		}
		r := bytes.NewReader(data)
		readPackedGUID(r) // caster item
		readPackedGUID(r) // caster
		var body struct {
			CastCount uint8
			SpellID   uint32
		}
		if binary.Read(r, binary.LittleEndian, &body) == nil && body.SpellID == spellRenegadeDrop {
			dropCast.Add(1)
		}
	})
	defer cancel()

	x, y, z, _ := bot.Pos()
	if res := castLanded(t, bot, spellVault, 0, 3); !res.Success {
		t.Fatalf("Vault refused: %s", e2eharness.SpellFailReasonName(res.FailReason))
	}
	time.Sleep(2 * time.Second)
	t.Logf("caltrop units within 60 yd after Vault: %d, SMSG_SPELL_GO for %d: %d",
		len(bot.UnitsByEntry(60, creatureCaltrop)), spellRenegadeDrop, dropCast.Load())

	for _, u := range bot.UnitsByEntry(60, creatureCaltrop) {
		if obj := bot.World.GetObject(u.GUID); obj != nil && obj.DistanceTo(x, y, z) < 8 {
			t.Logf("E2E_PASS: caltrop %s near the Vault start point", e2eharness.FormatGUID(u.GUID))
			return
		}
	}
	t.Errorf("E2E_FAIL: no caltrop within 8 yd of the Vault start point with Renegade (#901)")
}
