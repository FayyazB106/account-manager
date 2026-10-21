package report

import (
	"fmt"
	"strings"
	"time"
)

// Lang is the language an email is written in. It follows each admin's own UI
// language (stored on the server), falling back to English when Arabic support has
// been switched off in Settings → Configuration.
type Lang string

const (
	LangEN Lang = "en"
	LangAR Lang = "ar"
)

// Recipient is who an email is for: the address, the name to greet them with (the
// Arabic one when they read Arabic and have one) and the language to write in.
type Recipient struct {
	Name  string
	Email string
	Lang  Lang
}

// pick returns the English or Arabic wording. Keeping both side by side at the
// point of use makes it hard for one language to drift from the other.
func pick(l Lang, en, ar string) string {
	if l == LangAR {
		return ar
	}
	return en
}

// --- bidirectional text -------------------------------------------------------
//
// Arabic emails are right-to-left paragraphs that embed left-to-right tokens
// (service names, logins, ISO dates, "$12.50"). Without help, a mail client can
// reorder those tokens. rtlLine pins the paragraph direction; iso/ltr isolate an
// embedded run so it keeps its own order. All three are no-ops in English.

const (
	rlm = "‏" // right-to-left mark: makes the paragraph right-to-left
	fsi = "⁨" // first-strong isolate: direction follows the first strong letter
	lri = "⁦" // left-to-right isolate
	pdi = "⁩" // pop directional isolate
)

// rtlLine marks a whole line as a right-to-left paragraph (Arabic only).
func rtlLine(l Lang, s string) string {
	if l == LangAR && s != "" {
		return rlm + s
	}
	return s
}

// iso isolates user-supplied text (a name, service or login) whose direction is
// unknown, so it neither reorders nor is reordered by the text around it.
func iso(l Lang, s string) string {
	if l == LangAR && s != "" {
		return fsi + s + pdi
	}
	return s
}

// ltr isolates a token that is always left-to-right (a date or an amount).
func ltr(l Lang, s string) string {
	if l == LangAR && s != "" {
		return lri + s + pdi
	}
	return s
}

// --- words, plurals, dates ----------------------------------------------------

// appName is the product name as the app itself shows it in each language.
func appName(l Lang) string { return pick(l, "Account Manager", "مدير الحسابات") }

// arabicMonths are the Gregorian month names the app's Arabic UI uses.
var arabicMonths = [12]string{
	"يناير", "فبراير", "مارس", "أبريل", "مايو", "يونيو",
	"يوليو", "أغسطس", "سبتمبر", "أكتوبر", "نوفمبر", "ديسمبر",
}

// monthLabel renders "November 2026" / "نوفمبر 2026".
func monthLabel(l Lang, t time.Time) string {
	if l == LangAR {
		return fmt.Sprintf("%s %d", arabicMonths[t.Month()-1], t.Year())
	}
	return t.Format("January 2006")
}

// noun holds the forms of a counted noun. Arabic has distinct forms for 1, 2, 3-10,
// 11-99 and the rest; English only needs singular/plural.
type noun struct {
	enOne, enMany                        string
	arOne, arTwo, arFew, arMany, arOther string
}

var (
	nounPayment  = noun{"payment", "payments", "دفعة واحدة", "دفعتان", "دفعات", "دفعة", "دفعة"}
	nounPassword = noun{"password", "passwords", "كلمة مرور واحدة", "كلمتا مرور", "كلمات مرور", "كلمة مرور", "كلمة مرور"}
	nounDay      = noun{"day", "days", "يوم واحد", "يومان", "أيام", "يومًا", "يوم"}
)

// of renders a counted noun: "1 payment" / "5 payments" / "دفعتان" / "5 دفعات".
func (n noun) of(l Lang, count int) string {
	if l != LangAR {
		if count == 1 {
			return "1 " + n.enOne
		}
		return fmt.Sprintf("%d %s", count, n.enMany)
	}
	switch m := count % 100; {
	case count == 1:
		return n.arOne
	case count == 2:
		return n.arTwo
	case m >= 3 && m <= 10:
		return fmt.Sprintf("%d %s", count, n.arFew)
	case m >= 11:
		return fmt.Sprintf("%d %s", count, n.arMany)
	default:
		return fmt.Sprintf("%d %s", count, n.arOther)
	}
}

// whenText describes a due date relative to today: "due in 3 days" / "2 days
// overdue" / "due today" (and the Arabic equivalents the app's UI uses).
func whenText(l Lang, days int) string {
	if l == LangAR {
		switch {
		case days == 0:
			return "مستحقة اليوم"
		case days > 0:
			return "خلال " + nounDayDuration(days)
		default:
			return "متأخرة " + nounDayDuration(-days)
		}
	}
	switch {
	case days == 0:
		return "due today"
	case days > 0:
		return "due in " + nounDay.of(l, days)
	default:
		return nounDay.of(l, -days) + " overdue"
	}
}

// nounDayDuration renders a number of days as a duration ("يوم واحد", "يومين",
// "5 أيام"). Arabic uses the accusative dual "يومين" after "خلال"/"متأخرة", unlike
// the nominative "يومان" a counted noun takes on its own.
func nounDayDuration(days int) string {
	if days == 2 {
		return "يومين"
	}
	return nounDay.of(LangAR, days)
}

// greeting renders "Hi Alice," / "هلا Alice،".
func greeting(l Lang, name string) string {
	name = strings.TrimSpace(name)
	if l == LangAR {
		if name == "" {
			return "هلا،"
		}
		return "هلا " + iso(l, name) + "،"
	}
	if name == "" {
		return "Hi,"
	}
	return "Hi " + name + ","
}

// path writes a Settings location the way the app's menus read in each language.
func settingsSendEmails(l Lang) string {
	return pick(l, "Settings → Admin → Send Emails", "الإعدادات ← المشرف ← إرسال رسائل البريد")
}

// --- spreadsheet wording ------------------------------------------------------

// sheetText is the wording of the payments spreadsheet in one language. The Arabic
// terms are the ones the app's own Arabic UI uses for the same columns.
type sheetText struct {
	sheet, date, service, category, users, amount, paidIn, note, invoice, total string
}

func sheetTexts(l Lang) sheetText {
	if l == LangAR {
		return sheetText{"المدفوعات", "التاريخ", "الخدمة", "الفئة", "المستخدمون", "المبلغ", "تم الدفع بـ", "ملاحظة", "الفاتورة", "الإجمالي"}
	}
	return sheetText{"Payments", "Date", "Service", "Category", "Users", "Amount", "Paid in", "Note", "Invoice", "Total"}
}
