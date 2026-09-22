package main

import (
	"cmp"
	"fmt"
	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui"
)

type payment struct {
	ID, Status, Email string
	Amount            int
}

func newPaymentTable() ggui.Widget {
	rows := ggui.State([]payment{
		{"p1", "success", "ken99@example.com", 31600},
		{"p2", "success", "Abe45@example.com", 24200},
		{"p3", "processing", "Monserrat44@example.com", 83700},
		{"p4", "success", "Silas22@example.com", 87400},
		{"p5", "failed", "carmella@example.com", 72100},
		{"p6", "pending", "ada@example.com", 12500},
		{"p7", "success", "grace@example.com", 9900},
	})
	model := ui.NewTableModel(rows, func(p payment) string { return p.ID },
		ui.TextCol("Status", func(p payment) string { return p.Status }).W(110).Sortable(func(a, b payment) int { return cmp.Compare(a.Status, b.Status) }),
		ui.TextCol("Email", func(p payment) string { return p.Email }).Sortable(func(a, b payment) int { return cmp.Compare(a.Email, b.Email) }),
		ui.TextCol("Amount", func(p payment) string { return fmt.Sprintf("$%.2f", float64(p.Amount)/100) }).W(100).Right().Sortable(func(a, b payment) int { return cmp.Compare(a.Amount, b.Amount) }),
		ui.Col("", func(row ggui.Readable[payment]) ggui.Widget {
			return ui.Menu("…", ui.MenuItem("Remove payment", func() { id := row.Get().ID; ggui.Remove(rows, func(p payment) bool { return p.ID == id }) })).Name("Actions for " + row.Get().Email)
		}).Identified("actions").W(56),
	)
	model.SetPageSize(5)
	return ui.DataTable(model)
}
