import { useNavigate } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { useLayout } from "../components/Layout";
import { EventCalendar } from "../components/EventCalendar";

export function FinanceCalendar() {
  const { accounts, weekStart } = useLayout();
  const { t } = useTranslation();
  const navigate = useNavigate();
  return (
    <EventCalendar
      accounts={accounts}
      weekStart={weekStart}
      monthCount={12}
      columns={3}
      dividers="grid"
      title={t("finance.next12Months")}
      onOpen={(id) => navigate(`/account/${id}`)}
    />
  );
}
