package report

import (
	"fmt"
	"strings"
	"time"

	"github.com/FayyazB106/account-manager/internal/mailer"
)

// body builds an email's plain-text body line by line, marking each line as a
// right-to-left paragraph when the email is Arabic.
type body struct {
	l Lang
	b strings.Builder
}

func (w *body) line(s string)  { w.b.WriteString(rtlLine(w.l, s) + "\n") }
func (w *body) blank()         { w.b.WriteString("\n") }
func (w *body) String() string { return w.b.String() }

// subject prefixes the product name, as the app shows it in that language.
func subject(l Lang, rest string) string { return appName(l) + ": " + rest }

// who writes an account as "Service (login)", isolating each user-supplied part so
// an Arabic name or an ASCII email keeps its own order inside a right-to-left line.
func who(l Lang, service, login string) string {
	if login == "" {
		return iso(l, service)
	}
	return iso(l, service) + " (" + iso(l, login) + ")"
}

// optOutNote is the footer every scheduled email ends with.
func optOutNote(l Lang, what string) string {
	return what + " " + pick(l,
		"(Settings → Admin → Send Emails). You can turn them off there at any time.",
		"("+settingsSendEmails(l)+"). يمكنك إيقافها من هناك في أي وقت.")
}

// testMessage is the short message behind "Send test email".
func testMessage(to Recipient) mailer.Message {
	l := to.Lang
	w := &body{l: l}
	w.line(pick(l,
		"If you can read this, email is set up correctly. The emails you turn on in "+settingsSendEmails(l)+" will be sent to this address.",
		"إذا كنت تقرأ هذه الرسالة فقد تم إعداد البريد الإلكتروني بشكل صحيح. ستُرسل إلى هذا العنوان الرسائل التي تفعّلها من "+settingsSendEmails(l)+"."))
	return mailer.Message{To: []string{to.Email}, Subject: subject(l, pick(l, "test email", "بريد تجريبي")), Body: w.String(), RTL: l == LangAR}
}

// monthMessage is the monthly payment-history email (last month's spreadsheet attached).
func monthMessage(to Recipient, month time.Time, count int, xlsx []byte) mailer.Message {
	l := to.Lang
	label := monthLabel(l, month)
	w := &body{l: l}
	w.line(greeting(l, to.Name))
	w.blank()
	if count == 0 {
		w.line(pick(l,
			fmt.Sprintf("No payments were recorded in %s. The attached spreadsheet is empty.", label),
			fmt.Sprintf("لم تُسجَّل أي مدفوعات في %s. الجدول المرفق فارغ.", label)))
	} else {
		w.line(pick(l,
			fmt.Sprintf("Attached is your payment history for %s (%s).", label, nounPayment.of(l, count)),
			fmt.Sprintf("مرفق سجل مدفوعاتك لشهر %s (%s).", label, nounPayment.of(l, count))))
	}
	w.blank()
	w.line(optOutNote(l, pick(l,
		"You receive this because monthly reports are turned on for your profile",
		"تصلك هذه الرسالة لأن التقارير الشهرية مفعّلة في ملفك الشخصي")))
	return mailer.Message{
		To:      []string{to.Email},
		Subject: subject(l, pick(l, "payments for "+label, "مدفوعات "+label)),
		Body:    w.String(),
		RTL:     l == LangAR,
		Attachments: []mailer.Attachment{{
			Filename:    "payments-" + month.Format("2006-01") + ".xlsx",
			ContentType: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
			Data:        xlsx,
		}},
	}
}

// upcomingMessage is the start-of-month list of the month's upcoming payments.
func upcomingMessage(to Recipient, month time.Time, items []UpcomingItem, conv *Converter) mailer.Message {
	l := to.Lang
	label := monthLabel(l, month)
	total := 0.0
	for _, it := range items {
		total += it.Amount
	}
	w := &body{l: l}
	w.line(greeting(l, to.Name))
	w.blank()
	w.line(pick(l,
		fmt.Sprintf("These payments are coming up in %s: %s in total.", label, money(total, conv)),
		fmt.Sprintf("هذه المدفوعات القادمة في شهر %s، بإجمالي %s.", label, ltr(l, money(total, conv)))))
	w.blank()
	for _, it := range items {
		w.line(fmt.Sprintf("  - %s: %s - %s", ltr(l, it.Date.Format("2006-01-02")), who(l, it.Service, it.Login),
			ltr(l, fmt.Sprintf("$%.2f", it.Amount))))
	}
	w.blank()
	w.line(pick(l,
		"This is the same list as Finance → Monthly Costs in Account Manager.",
		"هذه هي القائمة نفسها في المالية ← التكاليف الشهرية داخل مدير الحسابات."))
	w.blank()
	w.line(optOutNote(l, pick(l,
		"You receive this at the start of each month because the upcoming payments email is turned on for your profile",
		"تصلك هذه الرسالة في بداية كل شهر لأن رسالة المدفوعات القادمة مفعّلة في ملفك الشخصي")))
	return mailer.Message{
		To: []string{to.Email},
		Subject: subject(l, pick(l,
			nounPayment.of(l, len(items))+" coming up in "+label,
			fmt.Sprintf("المدفوعات القادمة في %s (%d)", label, len(items)))),
		Body: w.String(),
		RTL:  l == LangAR,
	}
}

// reminderMessage is the daily due-payment reminder.
func reminderMessage(to Recipient, items []DueItem) mailer.Message {
	l := to.Lang
	w := &body{l: l}
	w.line(greeting(l, to.Name))
	w.blank()
	w.line(pick(l,
		"These payments are due within the next 7 days or are overdue:",
		"هذه الدفعات مستحقة خلال الأيام السبعة القادمة أو متأخرة:"))
	w.blank()
	for _, it := range items {
		text := fmt.Sprintf("  - %s: %s (%s)", iso(l, it.Service), ltr(l, it.Due.Format("2006-01-02")), whenText(l, it.Days))
		if it.Amount != nil && *it.Amount > 0 {
			text += " - " + ltr(l, fmt.Sprintf("$%.2f", *it.Amount))
		}
		w.line(text)
	}
	w.blank()
	w.line(pick(l,
		"Take action by updating account info with the latest payment details or deactivate account.",
		"يرجى اتخاذ إجراء بتحديث بيانات الحساب بأحدث تفاصيل الدفع أو إلغاء تفعيل الحساب."))
	w.blank()
	w.line(optOutNote(l, pick(l,
		"You receive this once a day while payments are due because payment reminders are turned on for your profile",
		"تصلك هذه الرسالة مرة واحدة يوميًا طالما هناك دفعات مستحقة لأن تذكيرات الدفع مفعّلة في ملفك الشخصي")))
	return mailer.Message{
		To: []string{to.Email},
		Subject: subject(l, pick(l,
			nounPayment.of(l, len(items))+" due",
			fmt.Sprintf("دفعات مستحقة (%d)", len(items)))),
		Body: w.String(),
		RTL:  l == LangAR,
	}
}

// rotationMessage is the daily password-rotation reminder.
func rotationMessage(to Recipient, items []RotationItem) mailer.Message {
	l := to.Lang
	w := &body{l: l}
	w.line(greeting(l, to.Name))
	w.blank()
	w.line(pick(l,
		"These passwords are due for rotation within the next 7 days or are overdue:",
		"كلمات المرور هذه يحين موعد تغييرها خلال الأيام السبعة القادمة أو متأخرة:"))
	w.blank()
	for _, it := range items {
		date := ltr(l, it.Due.Format("2006-01-02"))
		w.line(pick(l,
			fmt.Sprintf("  - %s: change by %s (%s)", who(l, it.Service, it.Login), date, whenText(l, it.Days)),
			fmt.Sprintf("  - %s: يجب تغييرها قبل %s (%s)", who(l, it.Service, it.Login), date, whenText(l, it.Days))))
	}
	w.blank()
	w.line(pick(l,
		"Take action by changing the password and saving the new one in Account Manager, or deactivate the account.",
		"يرجى تغيير كلمة المرور وحفظ الجديدة في مدير الحسابات، أو إلغاء تفعيل الحساب."))
	w.blank()
	w.line(optOutNote(l, pick(l,
		"You receive this once a day while passwords are due for rotation because password rotation reminders are turned on for your profile",
		"تصلك هذه الرسالة مرة واحدة يوميًا طالما هناك كلمات مرور يحين موعد تغييرها لأن تذكيرات تغيير كلمات المرور مفعّلة في ملفك الشخصي")))
	return mailer.Message{
		To: []string{to.Email},
		Subject: subject(l, pick(l,
			nounPassword.of(l, len(items))+" due for rotation",
			fmt.Sprintf("كلمات مرور تحتاج إلى تغيير (%d)", len(items)))),
		Body: w.String(),
		RTL:  l == LangAR,
	}
}
