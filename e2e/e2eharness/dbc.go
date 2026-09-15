package e2eharness

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/Gophercraft/core/format/dbc"
	"github.com/Gophercraft/core/vsn"
)

// SpellInfo contains human-readable metadata extracted directly from Spell.dbc.
type SpellInfo struct {
	ID          uint32
	Name        string
	Rank        string
	Description string
	ToolTip     string
}

// String returns a clean "Name (Rank)" or "Name" representation.
func (s *SpellInfo) String() string {
	if s == nil {
		return ""
	}
	if s.Rank != "" {
		return fmt.Sprintf("%s (%s)", s.Name, s.Rank)
	}
	return s.Name
}

var (
	spellTableOnce sync.Once
	spellTable     *dbc.Table
	spellTableErr  error
)

// initSpellTable loads Spell.dbc from E2E_DBC_PATH or default candidate locations.
func initSpellTable() {
	dbcPath := os.Getenv("E2E_DBC_PATH")
	var candidates []string

	if dbcPath != "" {
		// If the user passed the directory or the direct file path
		if strings.HasSuffix(strings.ToLower(dbcPath), ".dbc") {
			candidates = append(candidates, dbcPath)
		} else {
			candidates = append(candidates,
				filepath.Join(dbcPath, "Spell.dbc"),
				filepath.Join(dbcPath, "spell.dbc"),
			)
		}
	}

	// Default fallback paths
	candidates = append(candidates,
		`C:\ConquestOfAzerothCore\CoA-Repack\Data\dbc\Spell.dbc`,
		`..\CoA-Repack\Data\dbc\Spell.dbc`,
		`..\..\CoA-Repack\Data\dbc\Spell.dbc`,
		`Data\dbc\Spell.dbc`,
	)

	var targetFile string
	for _, c := range candidates {
		if fi, err := os.Stat(c); err == nil && !fi.IsDir() {
			targetFile = c
			break
		}
	}

	if targetFile == "" {
		spellTableErr = fmt.Errorf("Spell.dbc not found (check E2E_DBC_PATH)")
		return
	}

	f, err := os.Open(targetFile)
	if err != nil {
		spellTableErr = fmt.Errorf("open %s: %w", targetFile, err)
		return
	}
	defer f.Close()

	db := dbc.NewDB(vsn.Build(12340))
	table, err := db.Open("Spell", f)
	if err != nil {
		spellTableErr = fmt.Errorf("read %s: %w", targetFile, err)
		return
	}

	spellTable = table
}

func getDBCString(table *dbc.Table, offset uint32) string {
	if int(offset) >= len(table.StringBlock) {
		return ""
	}
	end := int(offset)
	for end < len(table.StringBlock) && table.StringBlock[end] != 0 {
		end++
	}
	return string(table.StringBlock[offset:end])
}

// GetSpell looks up a spell by ID directly in Spell.dbc via binary search.
func GetSpell(id uint32) (*SpellInfo, bool) {
	spellTableOnce.Do(initSpellTable)
	if spellTable == nil {
		return nil, false
	}

	recordSize := int(spellTable.Header.RecordSize)
	recordCount := int(spellTable.Header.RecordCount)

	// Records in WotLK / CoA DBC files are sorted ascending by ID (field 0)
	idx := sort.Search(recordCount, func(i int) bool {
		rec := spellTable.Records[i*recordSize : (i+1)*recordSize]
		return binary.LittleEndian.Uint32(rec[0:4]) >= id
	})

	if idx >= recordCount {
		return nil, false
	}

	rec := spellTable.Records[idx*recordSize : (idx+1)*recordSize]
	if binary.LittleEndian.Uint32(rec[0:4]) != id {
		return nil, false
	}

	// In 3.3.5a Spell.dbc (234 uint32 fields):
	// Field 136: SpellName (enUS)
	// Field 153: Rank / Subtext (enUS)
	// Field 170: Description (enUS)
	// Field 187: ToolTip (enUS)
	nameOffset := binary.LittleEndian.Uint32(rec[136*4 : 137*4])
	rankOffset := binary.LittleEndian.Uint32(rec[153*4 : 154*4])
	descOffset := binary.LittleEndian.Uint32(rec[170*4 : 171*4])
	tipOffset := binary.LittleEndian.Uint32(rec[187*4 : 188*4])

	return &SpellInfo{
		ID:          id,
		Name:        getDBCString(spellTable, nameOffset),
		Rank:        getDBCString(spellTable, rankOffset),
		Description: getDBCString(spellTable, descOffset),
		ToolTip:     getDBCString(spellTable, tipOffset),
	}, true
}

// DescribeSpell returns a human-friendly string for a spell ID (e.g. "Epoch (Rank 7) [501784]").
func DescribeSpell(id uint32) string {
	if info, ok := GetSpell(id); ok && info.Name != "" {
		if info.Rank != "" {
			return fmt.Sprintf("%s (%s) [%d]", info.Name, info.Rank, id)
		}
		return fmt.Sprintf("%s [%d]", info.Name, id)
	}
	return fmt.Sprintf("Spell %d", id)
}

// DescribeSpell returns a human-friendly string for a spell ID on ScenarioBot.
func (b *ScenarioBot) DescribeSpell(id uint32) string {
	return DescribeSpell(id)
}
