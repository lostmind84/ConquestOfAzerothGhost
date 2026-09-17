//go:build e2e

package multiclass_test

import (
	"fmt"
	"testing"

	_ "github.com/go-sql-driver/mysql"

	"github.com/azerothcore/AzerothGhost/e2e/e2eharness"
)

// Main project issue #147: a new Pyromancer's starting spells (Flare Bolt) are
// not on its action bar; every custom class had the same gap.
//
// A fresh level 1 character of each custom class is created and saved. Its
// stored action buttons must hold every active starting class spell, the
// passives excluded. Race is one the class allows.
//
//	go test -tags=e2e ./e2e/coa/multiclass -run TestCoA_CustomClassStartingActionBar -count=1 -v
func TestCoA_CustomClassStartingActionBar(t *testing.T) {
	for _, c := range []struct {
		name   string
		class  uint8
		race   uint8
		spells []uint32
	}{
		{"Barbarian", 12, 1, []uint32{801576, 804136}},
		{"WitchDoctor", 13, 1, []uint32{801670, 807037}},
		{"Felsworn", 14, 2, []uint32{801901, 802060}},
		{"WitchHunter", 15, 1, []uint32{802024, 804179}},
		{"Stormbringer", 16, 1, []uint32{500040, 804020}},
		{"KnightOfXoroth", 17, 2, []uint32{500904, 800168, 801016}},
		{"Guardian", 18, 1, []uint32{800311, 802197, 803417}},
		{"Templar", 19, 1, []uint32{801443, 803157}},
		{"Bloodmage", 20, 1, []uint32{500125, 802310}},
		{"Ranger", 21, 1, []uint32{500074, 800083, 802036}},
		{"Chronomancer", 22, 1, []uint32{801303, 804418}},
		{"Necromancer", 23, 1, []uint32{500970, 500985, 504868, 801722}},
		{"Pyromancer", 24, 10, []uint32{800790, 800792}},
		{"Cultist", 25, 1, []uint32{500720, 800413}},
		{"Starcaller", 26, 4, []uint32{800496, 801127, 801132}},
		{"SunCleric", 27, 1, []uint32{500143, 800231}},
		{"Tinker", 28, 1, []uint32{500239, 500549, 805351}},
		{"Venomancer", 29, 4, []uint32{800869, 805776}},
		{"Reaper", 30, 1, []uint32{500357, 500376, 573316}},
		{"Primalist", 31, 2, []uint32{500402, 800140}},
		{"Runemaster", 32, 1, []uint32{653022, 707141}},
	} {
		t.Run(c.name, func(t *testing.T) {
			bot := e2eharness.NewSolo(t, e2eharness.ScenarioOpts{
				Prefix: fmt.Sprintf("Bar%d", c.class), Race: c.race, Class: c.class,
			})
			for _, spell := range c.spells {
				if !bot.World.KnowsSpell(spell) {
					e2eharness.Preconditionf(t, "new %s does not know starting spell %d", c.name, spell)
				}
			}
			e2eharness.SaveCharacter(t, bot.World)

			buttons := map[uint32]uint8{}
			rows, err := bot.CharDB.Query(
				"SELECT button, action FROM character_action WHERE guid = ? AND type = 0", bot.World.CharGUID()&0xFFFFFFFF)
			if err != nil {
				e2eharness.HarnessFailf(t, "query character_action: %v", err)
			}
			defer rows.Close()
			for rows.Next() {
				var button uint8
				var action uint32
				if err := rows.Scan(&button, &action); err != nil {
					e2eharness.HarnessFailf(t, "scan character_action: %v", err)
				}
				buttons[action] = button
			}

			for _, spell := range c.spells {
				if button, ok := buttons[spell]; ok {
					t.Logf("E2E_PASS: new %s has spell %d on button %d", c.name, spell, button)
				} else {
					t.Errorf("E2E_FAIL: new %s has no action button for starting spell %d", c.name, spell)
				}
			}
		})
	}
}
