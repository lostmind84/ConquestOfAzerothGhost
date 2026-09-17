//go:build e2e

package summonspets_test

import (
	"bytes"
	"encoding/binary"
	"sync"
	"testing"
	"time"

	"github.com/azerothcore/AzerothGhost/e2e/e2eharness"
)

const (
	classWitchDoctor uint8 = 13

	specShadowhunter    uint32 = 4
	talentShadowhunter  uint32 = 4006
	spellShadowhunter   uint32 = 92086
	talentMimicWard     uint32 = 29301
	spellMimicWard      uint32 = 707162 // summons creature 300659
	talentReclamation   uint32 = 4505
	spellReclamation    uint32 = 806288
	creatureMimicWard   uint32 = 300659
	smsgSpellStart      uint16 = 0x0131
	smsgSpellGo         uint16 = 0x0132
	witchDoctorMimicLvl        = 20
)

var (
	maleficArrowRanks = []uint32{801674, 680908}
	loasBrewRanks     = []uint32{801670, 501198, 501199}
)

// castLog records the spell IDs each caster starts or launches (SMSG_SPELL_START / SMSG_SPELL_GO).
type castLog struct {
	mu    sync.Mutex
	casts map[uint64][]uint32
}

func watchCasts(t *testing.T, bot *e2eharness.ScenarioBot) *castLog {
	t.Helper()
	log := &castLog{casts: map[uint64][]uint32{}}
	cancel := bot.World.AddPacketHook(func(opcode uint16, data []byte) {
		if opcode != smsgSpellStart && opcode != smsgSpellGo {
			return
		}
		r := bytes.NewReader(data)
		readPackedGUID(r) // caster item or caster
		caster := readPackedGUID(r)
		var body struct {
			CastCount uint8
			SpellID   uint32
		}
		if binary.Read(r, binary.LittleEndian, &body) != nil {
			return
		}
		log.mu.Lock()
		log.casts[caster] = append(log.casts[caster], body.SpellID)
		log.mu.Unlock()
	})
	t.Cleanup(cancel)
	return log
}

func (l *castLog) cast(caster uint64, spell uint32) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, id := range l.casts[caster] {
		if id == spell {
			return true
		}
	}
	return false
}

// Main project issue #207: in the Shadowhunter specialization a Mimic Ward does not repeat Reclamation, Malefic
// Arrow or Loa's Brew. (The extra ward copy from the same report was fixed by #402.)
//
//	go test -tags=e2e ./e2e/coa/summonspets -run MimicWardRepeats -count=1 -v
func TestWitchDoctor_MimicWardRepeats(t *testing.T) {
	bot := newBot(t, "WdMim", e2eharness.RaceTroll, classWitchDoctor, witchDoctorMimicLvl)
	bot.SetSpecialization(t, specShadowhunter)
	for _, talent := range [][2]uint32{{talentShadowhunter, spellShadowhunter}, {talentMimicWard, spellMimicWard},
		{talentReclamation, spellReclamation}} {
		if !bot.World.KnowsSpell(talent[1]) {
			bot.SetTalentRank(t, talent[0], 1)
			waitKnows(t, bot, talent[1])
		}
	}
	arrow := knownRank(t, bot, maleficArrowRanks...)
	brew := knownRank(t, bot, loasBrewRanks...)
	waitKnows(t, bot, arrow)
	waitKnows(t, bot, brew)
	bot.CombatReadyFull(t)
	casts := watchCasts(t, bot)
	damage := watchSpellDamage(t, bot)

	x, y, z, m := bot.Pos()
	bot.Teleport(t, x+15, y, z, m)
	// A hostile creature, as in the report: the ward is not player-controlled, so neutral training dummies are
	// not valid targets for it.
	boar := spawnTarget(t, bot, creatureMottledBoar, witchDoctorMimicLvl-2, factionHostile)
	bot.GM(t, ".npc set react 0")
	bot.Teleport(t, x, y, z, m)
	bot.Face(t, boar)
	// Creatures cannot assist game masters, so the ward's Loa's Brew needs GM mode off.
	bot.GM(t, ".gm off")
	t.Cleanup(func() { bot.GM(t, ".gm on") })

	failed := 0
	for _, c := range []struct {
		name   string
		spell  uint32
		target uint64
	}{{"Malefic Arrow", arrow, boar}, {"Reclamation", spellReclamation, boar}, {"Loa's Brew", brew, bot.GUID}} {
		// A Mimic Ward lasts 12 seconds: drop a fresh one for each spell.
		_ = bot.World.SetTarget(bot.GUID)
		bot.GM(t, ".cooldown")
		time.Sleep(gcd)
		if res := castLanded(t, bot, spellMimicWard, 0, 3); !res.Success {
			t.Fatalf("precondition: Mimic Ward refused: %s", e2eharness.SpellFailReasonName(res.FailReason))
		}
		var ward uint64
		for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline) && ward == 0; time.Sleep(100 * time.Millisecond) {
			if wards := ownedUnits(bot, creatureMimicWard); len(wards) > 0 {
				ward = wards[len(wards)-1]
			}
		}
		if ward == 0 {
			t.Fatalf("precondition: no Mimic Ward appeared")
		}
		time.Sleep(gcd)
		bot.Face(t, boar)
		res := castLanded(t, bot, c.spell, c.target, 3)
		if !res.Success {
			t.Logf("%s %d refused: %s", c.name, c.spell, e2eharness.SpellFailReasonName(res.FailReason))
			failed++
			continue
		}
		time.Sleep(2 * time.Second)
		wardDamage := false
		for _, ev := range damage.snapshot() {
			if ev.attacker == ward && ev.target == boar {
				wardDamage = true
			}
		}
		if casts.cast(ward, c.spell) || wardDamage {
			t.Logf("E2E_PASS: the Mimic Ward repeated %s %d (#207)", c.name, c.spell)
		} else {
			t.Errorf("E2E_FAIL: the Mimic Ward did not repeat %s %d; ward casts %v (#207)", c.name, c.spell, casts.casts[ward])
		}
	}
	if failed > 0 {
		t.Errorf("precondition: %d owner cast(s) refused", failed)
	}
}
