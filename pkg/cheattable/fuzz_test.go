package cheattable

import "testing"

func FuzzParse(f *testing.F) {
	f.Add([]byte(`<CheatTable><CheatEntries><CheatEntry><ID>1</ID></CheatEntry></CheatEntries></CheatTable>`))
	f.Add([]byte(`<CheatTable Version="2"></CheatTable>`))
	f.Add([]byte(``))
	f.Add([]byte(`not xml`))
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 1<<16 {
			return
		}
		tbl, err := Parse(data)
		if err == nil && tbl != nil {
			_, _ = tbl.Marshal()
		}
	})
}
