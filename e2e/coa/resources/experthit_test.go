//go:build e2e

package resources_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/azerothcore/AzerothGhost/e2e/e2eharness"
)

const (
	spellExpertShotR1 uint32 = 705467 // aura 108 +15% and aura 333 +2% hit
	spellExpertShotR2 uint32 = 707889 // aura 108 +30% and aura 333 +4% hit (the rank named in #3173)
	expertShotLevel          = 55     // level reported in #3173

	// PLAYER_FIELD_COMBAT_RATING_1 = UNIT_END(0x0094) + 0x043B, then CR_HIT_MELEE/RANGED/SPELL = 5/6/7.
	playerFieldHitRatingMelee  uint16 = 0x04D4
	playerFieldHitRatingRanged uint16 = 0x04D5
	playerFieldHitRatingSpell  uint16 = 0x04D6
)

// Main project issue #3173: putting points into Expert Shot does not raise the Hit Rating value.
//
// Expert Shot carries aura 333 (SPELL_AURA_ASCENSION_MOD_HIT_CHANCE_ALL_PCT), which
// Unit::UpdateMeleeHitChances / UpdateRangedHitChances / UpdateSpellHitChances add straight into
// m_modMeleeHitChance, m_modRangedHitChance and m_modSpellHitChance. It is a hit *chance* percentage
// and never touches PLAYER_FIELD_COMBAT_RATING, which is where the Character Info "Hit Rating" line
// reads from, so that number cannot move however the talent behaves.
//
// The test records both: the effect amount the server holds for the passive, and the fact that the
// hit rating fields stay where they were. What the Character Info panel should display is client UI,
// outside this harness.
//
//	go test -tags=e2e ./e2e/coa/resources -run ExpertShot -count=1 -v
func TestWitchHunter_ExpertShotAppliesHitAura(t *testing.T) {
	bot := newBot(t, "WhShot", e2eharness.RaceOrc, classWitchHunter, expertShotLevel)
	_ = bot.World.SetTarget(bot.World.CharGUID()) // `.list auras` reads the selected unit

	self := bot.World.GetObject(bot.World.CharGUID())
	if self == nil {
		t.Fatalf("own player object not tracked")
	}
	beforeMelee := self.Value(playerFieldHitRatingMelee)
	beforeRanged := self.Value(playerFieldHitRatingRanged)
	beforeSpell := self.Value(playerFieldHitRatingSpell)

	for _, rank := range []struct {
		name     string
		spellID  uint32
		hitBonus int
	}{
		{"rank1", spellExpertShotR1, 2},
		{"rank2", spellExpertShotR2, 4},
	} {
		t.Run(rank.name, func(t *testing.T) {
			bot.Learn(t, rank.spellID)
			bot.FlushWorld(t)

			lines := gmChatLines(t, bot, fmt.Sprintf(".list auras id %d", rank.spellID), 2*time.Second)
			// "Target unit has N auras of type 333:" then "id: <spell> eff: 1 amount: <n>".
			want := fmt.Sprintf("id: %d eff: 1 amount: %d", rank.spellID, rank.hitBonus)
			var typeLine, amountLine string
			for _, line := range lines {
				if strings.Contains(line, "auras of type 333") {
					typeLine = line
				}
				if strings.Contains(line, want) {
					amountLine = line
				}
			}
			t.Logf("`.list auras id %d` answered %d line(s); type 333 line: %q", rank.spellID, len(lines), typeLine)
			if typeLine == "" || amountLine == "" {
				t.Errorf("E2E_FAIL: Expert Shot %d holds no aura 333 effect worth %d%% hit on the server (#3173); "+
					"lines: %q", rank.spellID, rank.hitBonus, lines)
				return
			}
			t.Logf("E2E_PASS: Expert Shot %d applies aura 333 with amount %d (%q)", rank.spellID, rank.hitBonus, amountLine)
		})
	}

	self = bot.World.GetObject(bot.World.CharGUID())
	afterMelee := self.Value(playerFieldHitRatingMelee)
	afterRanged := self.Value(playerFieldHitRatingRanged)
	afterSpell := self.Value(playerFieldHitRatingSpell)
	t.Logf("hit rating fields melee %d -> %d, ranged %d -> %d, spell %d -> %d; aura 333 is a hit chance "+
		"percentage and never writes these fields, so the Character Info number cannot move (#3173)",
		beforeMelee, afterMelee, beforeRanged, afterRanged, beforeSpell, afterSpell)
}
