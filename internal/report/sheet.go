package report

import (
	"strings"
	"time"

	"github.com/FayyazB106/account-manager/internal/store"
	"github.com/xuri/excelize/v2"
)

// PaymentsSheetName is the worksheet name used by every payments export.
const PaymentsSheetName = "Payments"

// seatList is the people on a payment's account, in the reader's language: an
// Arabic reader sees each seat's Arabic name when it has one (like the app does).
func seatList(p store.PaymentListItem, lang Lang) []string {
	var users []string
	for i, seat := range p.Seats {
		name := seat
		if lang == LangAR && i < len(p.SeatsAr) && p.SeatsAr[i] != nil && strings.TrimSpace(*p.SeatsAr[i]) != "" {
			name = *p.SeatsAr[i]
		}
		if strings.TrimSpace(name) != "" {
			users = append(users, name)
		}
	}
	return users
}

// WritePaymentsSheet fills an existing worksheet with the payments ledger: one
// "Amount (CODE)" column per active currency (each payment converted into it), a
// "Paid in" column naming the currency actually used, and a per-currency Total row.
// This is the single definition of the layout shared by the main Excel export and
// the monthly email (the Finance → Payment History export mirrors it client-side).
func WritePaymentsSheet(f *excelize.File, sheet string, payments []store.PaymentListItem, conv *Converter, lang Lang) {
	active := conv.Active()
	tx := sheetTexts(lang)
	if lang == LangAR {
		rtl := true // an Arabic sheet reads right to left
		_ = f.SetSheetView(sheet, 0, &excelize.ViewOptions{RightToLeft: &rtl})
	}

	header := []interface{}{tx.date, tx.service, tx.category, tx.users}
	for _, c := range active {
		header = append(header, tx.amount+" ("+c.Code+")")
	}
	header = append(header, tx.paidIn, tx.note, tx.invoice)
	_ = f.SetSheetRow(sheet, "A1", &header)

	for i, p := range payments {
		cat := ""
		if p.CategoryName != nil {
			cat = *p.CategoryName
		}
		note, invoice := "", ""
		if p.Note != nil {
			note = *p.Note
		}
		if p.AttachmentName != nil {
			invoice = *p.AttachmentName
		}
		row := []interface{}{p.PaidOn.Format("2006-01-02"), p.Service, cat, strings.Join(seatList(p, lang), ", ")}
		for _, c := range active {
			row = append(row, conv.AmountIn(p.Amount, p.Currency, c))
		}
		row = append(row, p.Currency, note, invoice)
		cell, _ := excelize.CoordinatesToCellName(1, i+2)
		_ = f.SetSheetRow(sheet, cell, &row)
	}

	total := []interface{}{tx.total, "", "", ""}
	for _, c := range active {
		sum := 0.0
		for _, p := range payments {
			usd := conv.ToUSD(p.Amount, p.Currency)
			if c.Code == "USD" {
				sum += usd
			} else {
				sum += usd * c.Rate
			}
		}
		total = append(total, Round2(sum))
	}
	total = append(total, "", "", "")
	cell, _ := excelize.CoordinatesToCellName(1, len(payments)+2)
	_ = f.SetSheetRow(sheet, cell, &total)
}

// BuildPaymentsWorkbook returns a one-sheet .xlsx containing the payments, worded
// in the given language.
func BuildPaymentsWorkbook(payments []store.PaymentListItem, conv *Converter, lang Lang) ([]byte, error) {
	f := excelize.NewFile()
	defer f.Close()
	name := sheetTexts(lang).sheet
	if err := f.SetSheetName("Sheet1", name); err != nil {
		return nil, err
	}
	WritePaymentsSheet(f, name, payments, conv, lang)
	buf, err := f.WriteToBuffer()
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// FilterByRange keeps payments whose date falls within [from, to], inclusive of
// both calendar days (the same rule as the Finance page's date filter).
func FilterByRange(payments []store.PaymentListItem, from, to time.Time) []store.PaymentListItem {
	lo, hi := from.Format("2006-01-02"), to.Format("2006-01-02")
	var out []store.PaymentListItem
	for _, p := range payments {
		day := p.PaidOn.Format("2006-01-02")
		if day >= lo && day <= hi {
			out = append(out, p)
		}
	}
	return out
}

// PreviousMonth returns the first and last day of the calendar month before
// `now`'s month, plus its "YYYY-MM" label.
func PreviousMonth(now time.Time) (from, to time.Time, label string) {
	first := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	to = first.AddDate(0, 0, -1)
	from = time.Date(to.Year(), to.Month(), 1, 0, 0, 0, 0, now.Location())
	return from, to, from.Format("2006-01")
}
