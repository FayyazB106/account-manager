package api

import (
	"archive/zip"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/FayyazB106/account-manager/internal/crypto"
	"github.com/FayyazB106/account-manager/internal/report"
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

	// Currency conversion is shared with the monthly email report (internal/report),
	// so every spreadsheet converts identically and follows the active currencies.
	allCurrencies, _ := s.Currencies.List(ctx)
	conv := report.NewConverter(allCurrencies)
	activeCcys := conv.Active()
	amountIn := conv.AmountIn

	f := excelize.NewFile()
	defer f.Close()
	firstSheetSet := false
	sheetsUsed := map[string]bool{}

	// Wrap style for the multi-line password-history column (one old password per
	// line, so a password containing commas/spaces stays unambiguous).
	wrapStyle, _ := f.NewStyle(&excelize.Style{Alignment: &excelize.Alignment{WrapText: true, Vertical: "top"}})

	// One "Amount (CODE)" column per active currency + a "Paid in" column, matching
	// the Finance exports. "Paid in" is the account's own currency.
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
				row = append(row, amountIn(*a.Amount, currencyOrUSDExport(a.Currency), c))
			} else {
				row = append(row, "")
			}
		}
		row = append(row, currencyOrUSDExport(a.Currency), nextPay, rotate, a.Status, len(a.Seats), strings.Join(users, ", "), notes)
		// Append to the next empty row on that sheet.
		rows, _ := f.GetRows(sheet)
		cell, _ := excelize.CoordinatesToCellName(1, len(rows)+1)
		f.SetSheetRow(sheet, cell, &row)
	}

	// Payments sheet: the full (unfiltered) payment ledger across all accounts,
	// appended after the per-category sheets. The layout lives in internal/report so
	// the monthly email attaches exactly the same sheet.
	if payments, perr := s.Payments.ListAll(ctx); perr == nil && len(payments) > 0 {
		if !firstSheetSet {
			f.SetSheetName("Sheet1", report.PaymentsSheetName)
			firstSheetSet = true
		} else {
			f.NewSheet(report.PaymentsSheetName)
		}
		report.WritePaymentsSheet(f, report.PaymentsSheetName, payments, conv, report.LangEN)
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

// currencyOrUSDExport is the account's currency for a spreadsheet row ("USD" if unset).
func currencyOrUSDExport(code string) string {
	if code == "" {
		return "USD"
	}
	return code
}
