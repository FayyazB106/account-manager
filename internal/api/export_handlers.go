package api

import (
	"archive/zip"
	"math"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/FayyazB106/account-manager/internal/crypto"
	"github.com/FayyazB106/account-manager/internal/store"
	"github.com/xuri/excelize/v2"
)

func billingLabelGo(months int, amount *float64) string {
	switch months {
	case 0:
		if amount != nil && *amount > 0 {
			return "One-time"
		}
		return "Free"
	case 1:
		return "Monthly"
	case 3:
		return "Quarterly"
	case 6:
		return "Every 6 months"
	case 12:
		return "Annual"
	default:
		return "Every " + strconv.Itoa(months) + " months"
	}
}

var invalidSheetChars = regexp.MustCompile(`[:\\/?*\[\]]`)

func sheetName(name string) string {
	n := invalidSheetChars.ReplaceAllString(name, " ")
	n = strings.TrimSpace(n)
	if n == "" {
		n = "Uncategorized"
	}
	if len(n) > 31 {
		n = n[:31]
	}
	return n
}

var unsafePath = regexp.MustCompile(`[<>:"/\\|?*]`)

func safeFolder(name string) string {
	n := unsafePath.ReplaceAllString(name, "_")
	n = strings.TrimSpace(n)
	if n == "" {
		n = "account"
	}
	return n
}

// exportAccounts handles GET /accounts/export?attachments=true|false.
// Builds an .xlsx with one sheet per category. With attachments=true, returns a
// .zip of that workbook plus a folder of decrypted files per account.
func (s *Server) exportAccounts(w http.ResponseWriter, r *http.Request) {
	withAttachments := r.URL.Query().Get("attachments") == "true"
	ctx := r.Context()

	accounts, err := s.Accounts.List(ctx)
	if err != nil {
		logErr("exportAccounts: list", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Active currencies (same source + selection the UI uses: the currency table,
	// active and with a usable rate). USD is the base (rate 1). Shared by the
	// per-category account sheets and the Payments sheet so both convert identically.
	var activeCcys []store.Currency
	rateOf := map[string]float64{}
	if all, cerr := s.Currencies.List(ctx); cerr == nil {
		for _, c := range all {
			rateOf[c.Code] = c.Rate
			if c.Active && (c.Code == "USD" || c.Rate > 0) {
				activeCcys = append(activeCcys, c)
			}
		}
	}
	round2 := func(v float64) float64 { return math.Round(v*100) / 100 }
	// toUSD / amountIn mirror the frontend's money.ts (amount ÷ rate → USD, then
	// × target rate), reading the identical rates from the currency table.
	toUSD := func(amount float64, code string) float64 {
		if code == "USD" {
			return amount
		}
		if r := rateOf[code]; r > 0 {
			return amount / r
		}
		return amount
	}
	amountIn := func(amount float64, code string, target store.Currency) float64 {
		usd := toUSD(amount, code)
		if target.Code == "USD" {
			return round2(usd)
		}
		return round2(usd * target.Rate)
	}

	f := excelize.NewFile()
	defer f.Close()
	firstSheetSet := false
	sheetsUsed := map[string]bool{}

	// Wrap style for the multi-line password-history column (one old password per
	// line, so a password containing commas/spaces stays unambiguous).
	wrapStyle, _ := f.NewStyle(&excelize.Style{Alignment: &excelize.Alignment{WrapText: true, Vertical: "top"}})

	// One "Amount (CODE)" column per active currency + a "Paid in" column, matching
	// the Finance exports. Account amounts are stored in USD base, so Paid in is USD.
	header := []interface{}{"Service", "Website", "Login", "Password", "Password history (Oldest to latest)", "Billing"}
	for _, c := range activeCcys {
		header = append(header, "Amount ("+c.Code+")")
	}
	header = append(header, "Paid in", "Next Payment", "Rotate (days)", "Status", "Seats", "Users", "Notes")

	for _, a := range accounts {
		cat := "Uncategorized"
		if a.CategoryName != nil {
			cat = *a.CategoryName
		}
		sheet := sheetName(cat)
		// Ensure a unique, existing sheet.
		if !sheetsUsed[sheet] {
			if !firstSheetSet {
				f.SetSheetName("Sheet1", sheet)
				firstSheetSet = true
			} else {
				f.NewSheet(sheet)
			}
			f.SetSheetRow(sheet, "A1", &header)
			// Column E = password history: wider + wrap so the stacked passwords read well.
			f.SetColWidth(sheet, "E", "E", 32)
			f.SetColStyle(sheet, "E", wrapStyle)
			sheetsUsed[sheet] = true
		}

		// Decrypt the current password.
		password := ""
		if wrapped, ciphertext, err := s.Accounts.GetSecret(ctx, a.ID); err == nil {
			if pt, err := crypto.EnvelopeDecrypt(s.MasterKeyValue(), &crypto.Envelope{WrappedDEK: wrapped, Ciphertext: ciphertext}); err == nil {
				password = string(pt)
			}
		}

		// Decrypt the previous passwords (oldest → latest), one per line.
		history := ""
		if secs, err := s.Accounts.ListHistorySecrets(ctx, a.ID); err == nil {
			var prev []string
			for _, hs := range secs {
				if pt, err := crypto.EnvelopeDecrypt(s.MasterKeyValue(), &crypto.Envelope{WrappedDEK: hs.WrappedDEK, Ciphertext: hs.Ciphertext}); err == nil {
					prev = append(prev, string(pt))
				}
			}
			history = strings.Join(prev, "\n")
		}

		website, login, notes := "", "", ""
		if a.Website != nil {
			website = *a.Website
		}
		if a.Login != nil {
			login = *a.Login
		}
		if a.Notes != nil {
			notes = *a.Notes
		}
		nextPay := ""
		if a.NextPaymentDate != nil {
			nextPay = a.NextPaymentDate.Format("2006-01-02")
		}
		rotate := ""
		if a.RotateEveryDays != nil {
			rotate = strconv.Itoa(*a.RotateEveryDays)
		}
		var users []string
		for _, seat := range a.Seats {
			if strings.TrimSpace(seat) != "" {
				users = append(users, seat)
			}
		}

		row := []interface{}{
			a.Service, website, login, password, history, billingLabelGo(a.BillingPeriodMonths, a.Amount),
		}
		for _, c := range activeCcys {
			if a.Amount != nil {
				row = append(row, amountIn(*a.Amount, "USD", c))
			} else {
				row = append(row, "")
			}
		}
		row = append(row, "USD", nextPay, rotate, a.Status, len(a.Seats), strings.Join(users, ", "), notes)
		// Append to the next empty row on that sheet.
		rows, _ := f.GetRows(sheet)
		cell, _ := excelize.CoordinatesToCellName(1, len(rows)+1)
		f.SetSheetRow(sheet, cell, &row)
	}

	// Payments sheet: the full (unfiltered) payment ledger across all accounts,
	// appended after the per-category sheets. Mirrors the Finance → Payments export:
	// one "Amount (CODE)" column per active currency (the payment converted into it),
	// plus a "Paid in" column naming the currency actually used.
	if payments, perr := s.Payments.ListAll(ctx); perr == nil && len(payments) > 0 {
		const ps = "Payments"
		if !firstSheetSet {
			f.SetSheetName("Sheet1", ps)
			firstSheetSet = true
		} else {
			f.NewSheet(ps)
		}

		phdr := []interface{}{"Date", "Service", "Category", "Users"}
		for _, c := range activeCcys {
			phdr = append(phdr, "Amount ("+c.Code+")")
		}
		phdr = append(phdr, "Paid in", "Note", "Invoice")
		f.SetSheetRow(ps, "A1", &phdr)

		for i, p := range payments {
			cat := ""
			if p.CategoryName != nil {
				cat = *p.CategoryName
			}
			var pusers []string
			for _, seat := range p.Seats {
				if strings.TrimSpace(seat) != "" {
					pusers = append(pusers, seat)
				}
			}
			note, invoice := "", ""
			if p.Note != nil {
				note = *p.Note
			}
			if p.AttachmentName != nil {
				invoice = *p.AttachmentName
			}
			prow := []interface{}{p.PaidOn.Format("2006-01-02"), p.Service, cat, strings.Join(pusers, ", ")}
			for _, c := range activeCcys {
				prow = append(prow, amountIn(p.Amount, p.Currency, c))
			}
			prow = append(prow, p.Currency, note, invoice)
			cell, _ := excelize.CoordinatesToCellName(1, i+2)
			f.SetSheetRow(ps, cell, &prow)
		}

		// Total row (per currency), matching the Finance → Payments export.
		totalRow := []interface{}{"Total", "", "", ""}
		for _, c := range activeCcys {
			sum := 0.0
			for _, p := range payments {
				usd := toUSD(p.Amount, p.Currency)
				if c.Code == "USD" {
					sum += usd
				} else {
					sum += usd * c.Rate
				}
			}
			totalRow = append(totalRow, round2(sum))
		}
		totalRow = append(totalRow, "", "", "")
		cell, _ := excelize.CoordinatesToCellName(1, len(payments)+2)
		f.SetSheetRow(ps, cell, &totalRow)
	}

	buf, err := f.WriteToBuffer()
	if err != nil {
		logErr("exportAccounts: workbook", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if !withAttachments {
		w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
		w.Header().Set("Content-Disposition", `attachment; filename="accounts.xlsx"`)
		w.Write(buf.Bytes())
		return
	}

	// Zip: workbook + a folder of decrypted attachments per account.
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="accounts-export.zip"`)
	zw := zip.NewWriter(w)
	defer zw.Close()

	if xw, err := zw.Create("accounts.xlsx"); err == nil {
		xw.Write(buf.Bytes())
	}

	usedFolders := map[string]int{}
	for _, a := range accounts {
		blobs, err := s.Attachments.ListBlobsByAccount(ctx, a.ID)
		if err != nil || len(blobs) == 0 {
			continue
		}
		// Include the login so same-named services (e.g. multiple "Adobe"
		// accounts) get distinct folders like "Adobe - user@example.com".
		base := a.Service
		if a.Login != nil && *a.Login != "" {
			base = a.Service + " - " + *a.Login
		}
		folder := safeFolder(base)
		if usedFolders[folder] > 0 {
			folder = folder + " (" + strconv.Itoa(usedFolders[folder]) + ")"
		}
		usedFolders[safeFolder(base)]++

		seen := map[string]int{}
		for _, b := range blobs {
			pt, err := crypto.EnvelopeDecrypt(s.MasterKeyValue(), &crypto.Envelope{WrappedDEK: b.WrappedDEK, Ciphertext: b.Ciphertext})
			if err != nil {
				continue
			}
			name := uniqueName(b.Filename, seen)
			if fw, err := zw.Create(folder + "/" + name); err == nil {
				fw.Write(pt)
			}
		}
	}
}
