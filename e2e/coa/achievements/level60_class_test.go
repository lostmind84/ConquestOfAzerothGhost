//go:build e2e

package achievements_test

import (
	"database/sql"
	"fmt"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"

	"github.com/azerothcore/AzerothGhost/e2e/e2eharness"
)

// jealous-sound/azerothcore-wotlk-coa issues #5377 (Reaper) and #5125 (Starcaller): reaching level 60
// on any CoA custom class awarded every custom class's "Level 60 <Class>" achievement (86000-86020),
// because their REACH_LEVEL criteria carried no achievement_criteria_data type 2
// (T_PLAYER_CLASS_RACE) row, so AchievementMgr::UpdateAchievementCriteria never gated them by class.
// Runtime reproduction on slot 2 (origin/main, unfixed) additionally showed the same defect on two more
// REACH_LEVEL, class-named achievement families in the same id space: "Realm First! Level 60 <Race>
// <Class>" (86021-86230, one row per CoA class per playable race) and "Realm First! Level 60 <Class>"
// (86500-86520, no race). Both are also flagged REALM_FIRST_REACH/REALM_FIRST_KILL in Achievement.dbc,
// so on a fresh database only the very first level-60 character of any class claims them all — but the
// plain 86000-86020 achievements are not realm-first (flags=0) and are awarded to every character that
// reaches level 60 as that class, so they are the only deterministic, repeatable part of this defect
// family and the only one this test requires to be present.
//
// Server fix: data/sql/updates/pending_db_world/rev_20260928_70_coa_level60_class_achievements.sql,
// which adds a type 2 (T_PLAYER_CLASS_RACE) row for every criteria id in the three families above,
// value1 = the CoA class id and (for the race+class family) value2 = the playable race id, both matched
// from each achievement's own Achievement.dbc name against ChrClasses.dbc / ChrRaces.dbc; 86008/86508/
// 86101-86110 "Son of Arugal" map to class 20 (Bloodmage) by elimination, the only CoA class id left
// unmatched. See .agents/plans/class-reaper/achievements.md.
//
// No SMSG_ACHIEVEMENT_EARNED handling exists in this harness (client/opcodes.go has no ACHIEVEMENT
// opcode), so this drives the server through the documented repro path (`.character level 60`, the
// same GM command e2eharness.SetLevel already uses, which calls Player::GiveLevel and its unconditional
// UpdateAchievementCriteria(ACHIEVEMENT_CRITERIA_TYPE_REACH_LEVEL)) and reads the result back from
// `character_achievement`, the table AchievementMgr persists completions to — the same DB-after-save
// pattern e2e/coa/multiclass uses for character_action. GM mode is turned off before leveling: REACH_LEVEL
// criteria updates are skipped while GM mode is on, confirmed by manual reproduction on slot 2. The
// character-save write lands asynchronously after `.save`, so the own achievement is polled rather than
// queried once.
//
//	go test -tags=e2e ./e2e/coa/achievements -run TestCoA_Level60ClassAchievement -count=1 -v
func TestCoA_Level60ClassAchievement(t *testing.T) {
	for _, c := range []struct {
		name  string
		class uint8
		race  uint8
		// own: every achievement id in 86000-86520 this character may legitimately hold.
		//   plain:        "Level 60 <Class>" (86000-86020) - always awarded, not realm-first.
		//   raceClass:    "Realm First! Level 60 <Race> <Class>" (86021-86230) for this bot's own race.
		//   classOnly:    "Realm First! Level 60 <Class>" (86500-86520).
		// raceClass/classOnly are realm-first (only the first level-60 character of that
		// class/race combo on the whole database earns them), so they are recorded but not required.
		plain, raceClass, classOnly uint32
	}{
		// Reaper (class 30) race 1 Human: race block order in Achievement.dbc is Draenei, Blood Elf,
		// Troll, Gnome, Tauren, Undead, Night Elf, Dwarf, Orc, Human (index 9 of the class-30 block
		// starting at 86201), giving 86210 "Realm First! Level 60 Human Reaper".
		{"Reaper", 30, 1, 86018, 86210, 86518},
		// Starcaller (class 26) race 4 Night Elf: index 6 of the class-26 block starting at 86161,
		// giving 86167 "Realm First! Level 60 Night Elf Starcaller".
		{"Starcaller", 26, 4, 86014, 86167, 86514},
	} {
		t.Run(c.name, func(t *testing.T) {
			own := map[uint32]string{
				c.plain:     "plain",
				c.raceClass: "realm-first race+class",
				c.classOnly: "realm-first class-only",
			}

			bot := e2eharness.NewSolo(t, e2eharness.ScenarioOpts{
				Prefix: fmt.Sprintf("Ach%d", c.class),
				Race:   c.race,
				Class:  c.class,
			})
			// REACH_LEVEL criteria updates are skipped while GM mode is on; GM mode defaults on
			// after login/setup, so it must be turned off before leveling reaches the criteria check.
			bot.GM(t, ".gm off")
			e2eharness.SetLevel(t, bot.World, 60)
			e2eharness.SaveCharacter(t, bot.World)

			guid := bot.World.CharGUID() & 0xFFFFFFFF

			if !waitForAchievement(t, bot.CharDB, guid, c.plain, 10*time.Second, 250*time.Millisecond) {
				t.Errorf("E2E_FAIL: level 60 %s did not earn its own achievement %d within 10s", c.name, c.plain)
			} else {
				t.Logf("E2E_PASS: level 60 %s earned its own achievement %d", c.name, c.plain)
			}

			rows, err := bot.CharDB.Query(
				"SELECT achievement FROM character_achievement WHERE guid = ? AND achievement BETWEEN 86000 AND 86520",
				guid)
			if err != nil {
				e2eharness.HarnessFailf(t, "query character_achievement: %v", err)
			}
			defer rows.Close()

			var extra []uint32
			for rows.Next() {
				var achievement uint32
				if err := rows.Scan(&achievement); err != nil {
					e2eharness.HarnessFailf(t, "scan character_achievement: %v", err)
				}
				if label, ok := own[achievement]; ok {
					t.Logf("E2E_PASS: level 60 %s holds its own %s achievement %d", c.name, label, achievement)
				} else {
					extra = append(extra, achievement)
				}
			}
			if err := rows.Err(); err != nil {
				e2eharness.HarnessFailf(t, "iterate character_achievement: %v", err)
			}

			if len(extra) > 0 {
				t.Errorf("E2E_FAIL: level 60 %s also earned other classes'/races' level 60 achievements: %v", c.name, extra)
			}
		})
	}
}

// waitForAchievement polls character_achievement until guid holds id or the deadline passes; the
// character save that persists a freshly completed achievement lands asynchronously after `.save`.
func waitForAchievement(t *testing.T, db *sql.DB, guid uint64, id uint32, timeout, interval time.Duration) bool {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		var found int
		err := db.QueryRow(
			"SELECT 1 FROM character_achievement WHERE guid = ? AND achievement = ?", guid, id).Scan(&found)
		if err == nil {
			return true
		}
		if err != sql.ErrNoRows {
			e2eharness.HarnessFailf(t, "query character_achievement: %v", err)
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(interval)
	}
}
