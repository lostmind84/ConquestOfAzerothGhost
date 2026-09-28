//go:build e2e

package crashes_test

import (
	"encoding/binary"
	"fmt"
	"testing"
	"time"

	"github.com/azerothcore/AzerothGhost/e2e/e2eharness"
)

func trialWireString(payload []byte, value string) []byte {
	payload = binary.LittleEndian.AppendUint32(payload, uint32(len(value)))
	return append(payload, value...)
}

func TestQuotedTrialIdActivatesAndDeactivatesBundle(t *testing.T) {
	bot := e2eharness.NewSolo(t, e2eharness.ScenarioOpts{
		Prefix: "TrialQ",
		Race:   e2eharness.RaceOrc,
		Class:  13,
	})
	guid := uint32(bot.World.CharGUID())
	trialId := fmt.Sprintf("quoted'%d", guid)
	t.Cleanup(func() {
		for _, table := range []string{"coa_custom_trial_active", "coa_character_challenge", "coa_custom_trial_entry", "coa_custom_trial"} {
			if table == "coa_character_challenge" {
				_, _ = bot.CharDB.Exec("DELETE FROM coa_character_challenge WHERE guid = ? AND challengeId = 8", guid)
			} else {
				_, _ = bot.CharDB.Exec("DELETE FROM "+table+" WHERE guid = ? AND trialId = ?", guid, trialId)
			}
		}
	})

	payload := trialWireString(nil, trialId)
	payload = trialWireString(payload, "Quoted trial")
	payload = trialWireString(payload, "")
	payload = trialWireString(payload, "")
	payload = binary.LittleEndian.AppendUint32(payload, 1)
	payload = binary.LittleEndian.AppendUint32(payload, 8)
	payload = binary.LittleEndian.AppendUint32(payload, 1)
	payload = trialWireString(payload, "")
	if err := bot.World.SendRawPacket(0x05A7, payload); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var count int
		if err := bot.CharDB.QueryRow("SELECT COUNT(*) FROM coa_custom_trial_entry WHERE guid = ? AND trialId = ? AND challengeId = 8", guid, trialId).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count == 1 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	var saved int
	if err := bot.CharDB.QueryRow("SELECT COUNT(*) FROM coa_custom_trial_entry WHERE guid = ? AND trialId = ? AND challengeId = 8", guid, trialId).Scan(&saved); err != nil {
		t.Fatal(err)
	}
	if saved != 1 {
		t.Fatalf("quoted trial was not saved: count=%d", saved)
	}

	if err := bot.World.SendRawPacket(0x05AD, trialWireString(nil, trialId)); err != nil {
		t.Fatal(err)
	}
	deadline = time.Now().Add(5 * time.Second)
	activated := false
	for time.Now().Before(deadline) {
		var count int
		if err := bot.CharDB.QueryRow("SELECT COUNT(*) FROM coa_character_challenge WHERE guid = ? AND challengeId = 8", guid).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count == 1 {
			activated = true
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !activated {
		t.Fatal("quoted trial did not activate its challenge")
	}

	if err := bot.World.SendRawPacket(0x05AF, nil); err != nil {
		t.Fatal(err)
	}
	deadline = time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var active, challenge int
		if err := bot.CharDB.QueryRow("SELECT COUNT(*) FROM coa_custom_trial_active WHERE guid = ?", guid).Scan(&active); err != nil {
			t.Fatal(err)
		}
		if err := bot.CharDB.QueryRow("SELECT COUNT(*) FROM coa_character_challenge WHERE guid = ? AND challengeId = 8", guid).Scan(&challenge); err != nil {
			t.Fatal(err)
		}
		if active == 0 && challenge == 0 {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("quoted trial did not deactivate its challenge")
}
