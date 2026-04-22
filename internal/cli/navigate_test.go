package cli

import "testing"

func TestEnsureCursorVisibleClampsOffsetWhenListShrinks(t *testing.T) {
	m := navigatorModel{
		rows: []navRow{
			{selectable: true},
			{selectable: true},
			{selectable: true},
			{selectable: true},
		},
		cursor: 3,
		offset: 3,
		height: 20,
	}

	m.ensureCursorVisible()

	if m.offset != 0 {
		t.Fatalf("offset = %d, want 0", m.offset)
	}
}

func TestEnsureCursorVisibleKeepsCursorInViewForLongLists(t *testing.T) {
	rows := make([]navRow, 20)
	for i := range rows {
		rows[i].selectable = true
	}
	m := navigatorModel{
		rows:   rows,
		cursor: 12,
		offset: 0,
		height: 10,
	}

	m.ensureCursorVisible()

	if m.offset != 8 {
		t.Fatalf("offset = %d, want 8", m.offset)
	}
}
