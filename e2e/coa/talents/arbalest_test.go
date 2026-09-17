//go:build e2e

package talents_test

import (
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/azerothcore/AzerothGhost/e2e/e2eharness"
)

const (
	// Crossbows: Spell.dbc 5011, verified via e2eharness.GetSpell against the live Spell.dbc (name "Crossbows"),
	// the same way spellProficiencyGuns (266, "Guns") is used for the gun-based Witch Hunter test above.
	spellProficiencyCrossbows uint32 = 5011

	// Crossbows the report says Witchbane needs equipped (from `.lookup item` on the live server).
	itemMakeshiftCrossbow uint32 = 419 // required level 3

	// Witchbane ranks (from `.lookup spell Witchbane` on the live server). 704342 is the internal family/channel
	// identifier the server code matches against, not a player-castable rank (Spell.dbc gives it no rank text).
	spellWitchbaneRank1 uint32 = 800165
	spellWitchbaneRank4 uint32 = 520213
	spellWitchbaneRank5 uint32 = 578297
	spellWitchbaneRank6 uint32 = 574319
	spellWitchbaneRank7 uint32 = 574320

	// Arbalest Mastery: Spell.dbc names 706240 "Passive" (tooltip: "Each subsequent shot of a Witchbane cast deals
	// $706241s1% more damage, up to a maximum of ...") and 706241 "Proc" — the stacking buff that actually carries
	// the per-shot multiplier read by the server's damage calculation.
	spellArbalestMastery         uint32 = 706240
	spellArbalestMasteryProgress uint32 = 706241
)

// witchbaneRanksHighToLow tries the highest rank first and falls back, so the test finds whichever rank a level 60
// Witch Hunter can actually cast instead of guessing a level gate.
var witchbaneRanksHighToLow = []uint32{
	spellWitchbaneRank7, spellWitchbaneRank6, spellWitchbaneRank5, spellWitchbaneRank4, spellWitchbaneRank1,
}

// backOffFromTarget teleports the bot to ~yards away from the target's *current* position and faces it, then
// returns the distance that was measured beforehand. Witchbane needs range: even a neutral-faction target still
// closes in after being hit, and the cast then refuses with SPELL_FAILED_TOO_CLOSE (confirmed empirically — see
// TestWitchHunter_ArbalestMasteryAuraPlacement's history). Call this before every cast, not just the first.
func backOffFromTarget(t *testing.T, bot *e2eharness.ScenarioBot, targetGUID uint64, yards float32) float64 {
	t.Helper()
	obj := bot.World.GetObject(targetGUID)
	if obj == nil {
		t.Fatalf("precondition: target %d not tracked", targetGUID)
	}
	tx, ty, tz := obj.InterpolatedPosition()
	bx, by, _, mapID := bot.Pos()
	dx, dy := float64(bx-tx), float64(by-ty)
	dist := math.Hypot(dx, dy)
	if dist < 1 {
		dx, dy, dist = 1, 0, 1
	}
	nx, ny := dx/dist, dy/dist
	bot.Teleport(t, tx+float32(nx*float64(yards)), ty+float32(ny*float64(yards)), tz, mapID)
	time.Sleep(settle)
	bot.Face(t, targetGUID)
	return dist
}

// spawnPersistentNoCleanup is SpawnPersistent (`.npc add`, DB-backed, not a 120s temp summon) without the
// t.Cleanup despawn it normally registers. Use only for a fixture that must outlive this Go process, such as the
// lab-client setup target in TestArbalestLabSetup.
func spawnPersistentNoCleanup(t *testing.T, bot *e2eharness.ScenarioBot, entry uint32, timeout time.Duration) (liveGUID uint64, spawnID uint32) {
	t.Helper()
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	bot.DespawnNearbyEntry(t, entry, 100) // clear any stale fixture left by an earlier run
	known := map[uint64]struct{}{}
	for _, u := range bot.UnitsByEntry(100, entry) {
		known[u.GUID] = struct{}{}
	}
	bot.GM(t, ".gm on")
	bot.GM(t, fmt.Sprintf(".npc add %d", entry))
	spawnID = bot.CaptureCreatureSpawnID(t, entry)
	newOnes := bot.WaitNewUnits(t, known, []uint32{entry}, timeout)
	if len(newOnes) == 0 {
		t.Fatalf("precondition: entry %d not in object cache after .npc add", entry)
	}
	liveGUID = newOnes[0].GUID
	best := float32(1e9)
	px, py, pz, _ := bot.Pos()
	for _, u := range newOnes {
		obj := bot.World.GetObject(u.GUID)
		if obj == nil || !obj.HasKnownPosition() {
			continue
		}
		ox, oy, oz := obj.InterpolatedPosition()
		d := e2eharness.Distance3D(px, py, pz, ox, oy, oz)
		if d < best {
			best = d
			liveGUID = u.GUID
		}
	}
	t.Logf("spawnPersistentNoCleanup entry=%d live=0x%X dbSpawn=%d dist=%.1f (no auto-despawn: left for the lab client)",
		entry, liveGUID, spawnID, best)
	return liveGUID, spawnID
}

// Main project issue #3935: Arbalest Mastery is being displayed wrong. Reported from the in-game form (class id 15,
// level 60): "It shows the debuff on you, not the enemy, damage amplification is correct though." Arbalest Mastery
// is a passive: "Each subsequent shot of a Witchbane cast deals 15% more damage, up to a maximum of 150%."
//
//	go test -tags=e2e ./e2e/coa/talents -run Arbalest -count=1 -v
func TestWitchHunter_ArbalestMasteryAuraPlacement(t *testing.T) {
	bot := newBot(t, "ArbMast", e2eharness.RaceHuman, classWitchHunter, 60)

	// Witchbane needs a ranged weapon: #3935's own repro (a plain Warrior with .learn, no crossbow) got
	// SPELL_FAILED_BAD_TARGETS / "Must have the proper item equipped". Give the bot proficiency and a crossbow,
	// mirroring TestWitchHunter_BurrowBoltPulls's Learn(gun proficiency)+equip(gun) pattern for this class.
	bot.Learn(t, spellProficiencyCrossbows)
	equip(t, bot, itemMakeshiftCrossbow)

	if !bot.World.KnowsSpell(spellArbalestMastery) {
		t.Logf("level 60 Witch Hunter does not know Arbalest Mastery %d, learning it", spellArbalestMastery)
		bot.Learn(t, spellArbalestMastery)
	}
	for _, r := range witchbaneRanksHighToLow {
		bot.Learn(t, r)
	}

	thug := spawnTarget(t, bot, creatureDefiasThug, 60)
	// Neutral like TestWitchHunter_BurrowBoltPulls sets it: a live hostile Defias Thug walks into melee range once
	// hit, and Witchbane (a ranged crossbow ability) then refuses with SPELL_FAILED_TOO_CLOSE on the following casts
	// (confirmed by an earlier run of this test without this line).
	bot.GM(t, ".npc set faction 7")
	bot.CombatReadyFull(t)

	// Witchbane needs range; even neutral, the thug still closes in after being hit, so re-establish 15 yd from its
	// *current* position before every cast rather than once (confirmed necessary by an earlier run: cast 1 landed
	// at 15 yd, casts 2-3 then refused with SPELL_FAILED_TOO_CLOSE once the thug had closed the gap).
	dist := backOffFromTarget(t, bot, thug, 15)
	t.Logf("distance to target before cast: was %.1f yd, now ~15 yd", dist)

	// Find the highest Witchbane rank the bot can actually cast at level 60.
	var rank uint32
	var lastRefusal e2eharness.SpellCastResult
	for _, candidate := range witchbaneRanksHighToLow {
		res := castLanded(t, bot, candidate, thug, 2)
		if res.Success {
			rank = candidate
			break
		}
		lastRefusal = res
	}
	if rank == 0 {
		t.Fatalf("E2E_FAIL: no Witchbane rank could be cast by a level 60 Witch Hunter with a crossbow equipped (#3935); "+
			"last refusal: %s", e2eharness.SpellFailReasonName(lastRefusal.FailReason))
	}
	t.Logf("casting %s", bot.DescribeSpell(rank))

	// Two more casts (three total) so Arbalest Mastery's per-shot stacks have a chance to build and apply.
	for cast := 2; cast <= 3; cast++ {
		time.Sleep(3 * time.Second) // let the previous channel finish firing its shots
		d := backOffFromTarget(t, bot, thug, 15)
		t.Logf("distance to target before cast: was %.1f yd, now ~15 yd", d)
		if res := castLanded(t, bot, rank, thug, 3); !res.Success {
			t.Fatalf("E2E_FAIL: Witchbane (%s) refused on cast %d/3: %s (#3935)",
				bot.DescribeSpell(rank), cast, e2eharness.SpellFailReasonName(res.FailReason))
		}
	}
	time.Sleep(3 * time.Second) // let the last channel finish before reading auras
	time.Sleep(settle)

	selfGUID := bot.World.CharGUID()
	selfAuras := serverAuras(t, bot, selfGUID)
	enemyAuras := serverAuras(t, bot, thug)
	t.Logf("server auras on the Witch Hunter (self, %d): %v", selfGUID, selfAuras)
	t.Logf("server auras on the target (%d): %v", thug, enemyAuras)
	t.Logf("client HasAura on self: mastery(%d)=%v progress(%d)=%v",
		spellArbalestMastery, bot.HasAura(spellArbalestMastery),
		spellArbalestMasteryProgress, bot.HasAura(spellArbalestMasteryProgress))
	t.Logf("client UnitHasAura on target: mastery(%d)=%v progress(%d)=%v",
		spellArbalestMastery, bot.UnitHasAura(thug, spellArbalestMastery),
		spellArbalestMasteryProgress, bot.UnitHasAura(thug, spellArbalestMasteryProgress))

	if len(selfAuras) == 0 && len(enemyAuras) == 0 {
		t.Errorf("E2E_FAIL: no aura at all on the Witch Hunter or the target after 3 Witchbane casts (#3935)")
		return
	}

	// Log every aura seen on either unit so the real id behind the report is visible, not just the two we expect.
	for id := range selfAuras {
		t.Logf("aura %d present on the Witch Hunter (server)", id)
	}
	for id := range enemyAuras {
		t.Logf("aura %d present on the target (server)", id)
	}

	_, masteryOnBotServer := selfAuras[spellArbalestMastery]
	_, masteryOnTargetServer := enemyAuras[spellArbalestMastery]
	masteryOnBot := masteryOnBotServer || bot.HasAura(spellArbalestMastery)
	masteryOnTarget := masteryOnTargetServer || bot.UnitHasAura(thug, spellArbalestMastery)

	_, progressOnBotServer := selfAuras[spellArbalestMasteryProgress]
	_, progressOnTargetServer := enemyAuras[spellArbalestMasteryProgress]
	progressOnBot := progressOnBotServer || bot.HasAura(spellArbalestMasteryProgress)
	progressOnTarget := progressOnTargetServer || bot.UnitHasAura(thug, spellArbalestMasteryProgress)

	if !masteryOnBot && !masteryOnTarget && !progressOnBot && !progressOnTarget {
		t.Errorf("E2E_FAIL: neither Arbalest Mastery (%d) nor its stacking proc (%d) appeared on the Witch Hunter "+
			"or the target after 3 Witchbane casts (#3935)", spellArbalestMastery, spellArbalestMasteryProgress)
		return
	}

	// The issue's claim: the mastery aura must be on the target, not on the caster.
	failed := false
	if masteryOnBot && !masteryOnTarget {
		t.Errorf("E2E_FAIL: Arbalest Mastery (%d) is on the Witch Hunter, not on the target (#3935)", spellArbalestMastery)
		failed = true
	} else if masteryOnTarget {
		t.Logf("E2E_PASS: Arbalest Mastery (%d) is on the target", spellArbalestMastery)
	}
	if progressOnBot && !progressOnTarget {
		t.Errorf("E2E_FAIL: Arbalest Mastery Progress (%d), the stacking aura that carries the per-shot damage "+
			"bonus, is on the Witch Hunter, not on the target (#3935)", spellArbalestMasteryProgress)
		failed = true
	} else if progressOnTarget {
		t.Logf("E2E_PASS: Arbalest Mastery Progress (%d) is on the target", spellArbalestMasteryProgress)
	}
	if !failed {
		t.Logf("E2E_PASS: no Arbalest Mastery aura landed on the Witch Hunter instead of the target")
	}
}

// labAccount/labPassword/labChar are the fixed identity coa-client-lab logs into to look at #3935 directly. Unlike
// newBot's random-prefixed throwaway accounts, this one must be stable across runs and must not be torn down by
// t.Cleanup, since the real client logs in well after this Go process exits.
const (
	labAccount  = "labarba"
	labPassword = "labarba"
	labChar     = "Labarba"
	labLevel    = 60
)

// TestArbalestLabSetup prepares the "labarba" account/character so the coa-client-lab game client can look at
// issue #3935 itself: it applies the exact setup TestWitchHunter_ArbalestMasteryAuraPlacement proved works (crossbow
// proficiency + equip, Arbalest Mastery, a castable Witchbane rank) and spawns the same fixture target at castable
// range, but it does not cast — the lab client does that — and it logs the bot out cleanly (CMSG_LOGOUT_REQUEST,
// not a raw socket close) so the account is free for the client's own login right after.
//
// Guarded behind its own -run: a normal package run (`go test ./e2e/coa/talents`) must not touch this persistent
// account or its target, which is why this is a separate test from TestWitchHunter_ArbalestMasteryAuraPlacement.
//
//	go test -tags=e2e ./e2e/coa/talents -run ArbalestLabSetup -count=1 -v
func TestArbalestLabSetup(t *testing.T) {
	authDB, charDB := e2eharness.OpenTestDBs(t)
	if err := e2eharness.EnsureAccount(authDB, labAccount, labPassword); err != nil {
		t.Fatalf("ensure account %s: %v", labAccount, err)
	}
	if err := e2eharness.SetGM(authDB, labAccount, 3); err != nil {
		t.Fatalf("set gm %s: %v", labAccount, err)
	}

	session, err := e2eharness.LoginBot(t, e2eharness.LoginOptions{
		User:     labAccount,
		Password: labPassword,
		CharName: labChar,
		Race:     e2eharness.RaceHuman,
		Class:    classWitchHunter,
	})
	if err != nil {
		t.Fatalf("login %s: %v", labAccount, err)
	}
	bot := &e2eharness.ScenarioBot{
		Session: session,
		AuthDB:  authDB,
		CharDB:  charDB,
		Ident: e2eharness.BotIdent{
			Account: labAccount, CharName: labChar, Race: e2eharness.RaceHuman, Class: classWitchHunter,
		},
	}
	// Safety net only: the test logs out gracefully itself below. This just guarantees the socket is not left open
	// if a Fatalf above/below exits early.
	t.Cleanup(func() { bot.Close() })
	if session.Name != labChar {
		t.Logf("NOTE: server assigned character name %q instead of requested %q (a character named %q already "+
			"exists elsewhere) — use %q for everything below", session.Name, labChar, labChar, session.Name)
	}
	charName := session.Name

	bot.TeleportPad(t, e2eharness.PackagePad(t))
	e2eharness.EnableGM(t, bot.World)
	e2eharness.SetLevel(t, bot.World, labLevel) // logs "level set to N" itself

	// Same setup TestWitchHunter_ArbalestMasteryAuraPlacement proved works. No explicit spec selection was needed
	// there (Arbalest Mastery is a base passive, not talent-gated), so none is done here either.
	bot.Learn(t, spellProficiencyCrossbows)
	equip(t, bot, itemMakeshiftCrossbow)
	if !bot.World.KnowsSpell(spellArbalestMastery) {
		bot.Learn(t, spellArbalestMastery)
	}
	for _, r := range witchbaneRanksHighToLow {
		bot.Learn(t, r)
	}
	// TestWitchHunter_ArbalestMasteryAuraPlacement already proved, twice, that a level 60 Witch Hunter set up this
	// way can cast Rank 7 (574320) — the highest rank — successfully. This test does not cast (the lab client
	// does), so it reports that proven rank instead of re-discovering it here.
	const provenRank = spellWitchbaneRank7
	t.Logf("Witchbane rank to cast (proven castable at level %d by the sibling test): %s",
		labLevel, bot.DescribeSpell(provenRank))

	worldDB, err := e2eharness.OpenWorldDB()
	if err != nil {
		t.Fatalf("open world db: %v", err)
	}
	defer worldDB.Close()
	var creatureName string
	if err := worldDB.QueryRow(`SELECT name FROM creature_template WHERE entry = ?`, creatureDefiasThug).
		Scan(&creatureName); err != nil {
		t.Fatalf("creature_template name for entry %d: %v", creatureDefiasThug, err)
	}

	// Persistent (`.npc add`), not the auto-despawned fixture the other test uses: this target must still be there
	// when the lab client logs in later. Same level and neutral faction as the proven setup.
	thug, spawnID := spawnPersistentNoCleanup(t, bot, creatureDefiasThug, 15*time.Second)
	bot.GM(t, fmt.Sprintf(".npc set level %d", labLevel))
	bot.GM(t, ".npc set faction 7")

	// Leave the character ~15 yd from the target: melee range refuses Witchbane with SPELL_FAILED_TOO_CLOSE (see
	// TestWitchHunter_ArbalestMasteryAuraPlacement's history).
	dist := backOffFromTarget(t, bot, thug, 15)
	t.Logf("distance to target before backing off: %.1f yd", dist)

	bot.GM(t, ".gm off") // leave the character in a normal (non-GM-mode) state for the client to pick up
	bot.Save(t)

	x, y, z, mapID := bot.Pos()
	t.Logf("E2E_PASS: labarba setup ready for the client")
	t.Logf("account: %s", labAccount)
	t.Logf("character: %s", charName)
	t.Logf("creature: %s (entry %d, live guid 0x%X, db spawn id %d)", creatureName, creatureDefiasThug, thug, spawnID)
	t.Logf("Witchbane rank to cast: %s", bot.DescribeSpell(provenRank))
	t.Logf("character position: x=%.1f y=%.1f z=%.1f map=%d", x, y, z, mapID)

	// Graceful logout (CMSG_LOGOUT_REQUEST), not a raw socket close, so the account is not left "online" and
	// refuses the lab client's login right after.
	_ = bot.World.SendLogout()
	if err := bot.World.WaitForLogout(30 * time.Second); err != nil {
		t.Logf("logout wait: %v (continuing with Close)", err)
	}
	bot.Close()
}
