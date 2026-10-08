import { useEffect, useState } from "react";
import { Navigate, Route, Routes } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { hasToken, restoreSession, getAppState, type AppState } from "./api";
import { Layout } from "./components/Layout";
import { LoginPage } from "./pages/LoginPage";
import { SetupPage } from "./pages/SetupPage";
import { UnlockPage } from "./pages/UnlockPage";
import { HomePage } from "./pages/HomePage";
import { AccountsPage } from "./pages/AccountsPage";
import {
  AccountLayout,
  AccountGeneralTab,
  AccountPasswordsTab,
  AccountPaymentsTab,
  AccountAttachmentsTab,
} from "./pages/AccountLayout";
import { HealthPage } from "./pages/HealthPage";
import { AuthenticatorPage } from "./pages/AuthenticatorPage";
import { FinanceLayout } from "./pages/FinanceLayout";
import { FinanceAccounts } from "./pages/FinanceAccounts";
import { FinanceCalendar } from "./pages/FinanceCalendar";
import { FinanceGraph } from "./pages/FinanceGraph";
import { FinancePayments } from "./pages/FinancePayments";
import { SettingsLayout } from "./pages/SettingsLayout";
import { CategoriesSettings } from "./pages/CategoriesSettings";
import { AdminSettings } from "./pages/AdminSettings";
import { UsersSettings } from "./pages/UsersSettings";
import { ConfigurationSettings } from "./pages/ConfigurationSettings";
import { DeploymentSettings } from "./pages/DeploymentSettings";
import { AuditSettings } from "./pages/AuditSettings";

// RequireAuth redirects to /login when there is no session token.
function RequireAuth({ children }: { children: React.ReactNode }) {
  return hasToken() ? <>{children}</> : <Navigate to="/login" replace />;
}

export default function App() {
  const { t } = useTranslation();
  // Decide onboarding state first: first-run setup, locked (needs passphrase), or
  // ready. When ready, also try to restore a session from the refresh cookie
  // (the access token is memory-only and gone after a reload).
  const [state, setState] = useState<AppState | null>(null);
  useEffect(() => {
    getAppState().then(async (st) => {
      if (st === "ready") await restoreSession();
      setState(st);
    });
  }, []);

  if (state === null) {
    return <div className="centered"><p className="muted">{t("common.loading")}</p></div>;
  }
  if (state === "setup") {
    return <SetupPage onDone={() => setState("ready")} />;
  }
  if (state === "locked") {
    return <UnlockPage onUnlocked={() => setState("ready")} />;
  }

  return (
    <Routes>
      <Route path="/login" element={<LoginPage />} />

      <Route
        element={
          <RequireAuth>
            <Layout />
          </RequireAuth>
        }
      >
        <Route path="/" element={<HomePage />} />
        <Route path="/accounts" element={<AccountsPage />} />
        <Route path="/c/:categoryId" element={<AccountsPage />} />
        <Route path="/account/:id" element={<AccountLayout />}>
          <Route index element={<Navigate to="general" replace />} />
          <Route path="general" element={<AccountGeneralTab />} />
          <Route path="passwords" element={<AccountPasswordsTab />} />
          <Route path="payments" element={<AccountPaymentsTab />} />
          <Route path="attachments" element={<AccountAttachmentsTab />} />
        </Route>
        <Route path="/health" element={<HealthPage />} />
        <Route path="/authenticator" element={<AuthenticatorPage />} />
        <Route path="/finance" element={<FinanceLayout />}>
          <Route index element={<Navigate to="accounts" replace />} />
          <Route path="accounts" element={<FinanceAccounts />} />
          <Route path="payments" element={<FinancePayments />} />
          <Route path="calendar" element={<FinanceCalendar />} />
          <Route path="graph" element={<FinanceGraph />} />
        </Route>
        <Route path="/settings" element={<SettingsLayout />}>
          <Route index element={<Navigate to="admin" replace />} />
          <Route path="categories" element={<CategoriesSettings />} />
          <Route path="admin" element={<AdminSettings />} />
          <Route path="users" element={<UsersSettings />} />
          <Route path="configuration" element={<ConfigurationSettings />} />
          <Route path="deployment" element={<DeploymentSettings />} />
          <Route path="audit" element={<AuditSettings />} />
        </Route>
      </Route>

      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
  );
}
