package e2eharness

import "testing"

func TestSetConfigValues(t *testing.T) {
	content := "# Rate.XP.Kill = 5\r\nRate.XP.Kill      = 1\r\nRate.XP.Quest     = 1\r\nRate.XP.Quest.DF  = 1\r\n"
	got, err := SetConfigValues(content, map[string]string{"Rate.XP.Kill": "10", "Rate.XP.Quest": "3"})
	if err != nil {
		t.Fatal(err)
	}
	want := "# Rate.XP.Kill = 5\r\nRate.XP.Kill = 10\r\nRate.XP.Quest = 3\r\nRate.XP.Quest.DF  = 1\r\n"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestSetConfigValuesRequiresExactlyOneSetting(t *testing.T) {
	for name, content := range map[string]string{
		"missing":   "Rate.XP.Quest = 1\n",
		"commented": "#Rate.XP.Kill = 1\n",
		"duplicate": "Rate.XP.Kill = 1\nRate.XP.Kill = 2\n",
	} {
		if _, err := SetConfigValues(content, map[string]string{"Rate.XP.Kill": "10"}); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}
