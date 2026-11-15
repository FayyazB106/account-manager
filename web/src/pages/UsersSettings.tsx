import { useEffect, useMemo, useRef, useState, type FormEvent } from "react";
import { Link } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { listPeople, addPerson, updatePerson, deletePerson, deletePeople, importPeople, type Person, type AccountListItem } from "../api";
import { confirmDialog, toast } from "../alerts";
import { useLayout } from "../components/Layout";
import { Button } from "../components/Button";
import { IconButton } from "../components/IconButton";
import { CircleButton } from "../components/CircleButton";
import { Field } from "../components/Field";
import { Modal } from "../components/Modal";
import { ImportUsersModal } from "../components/ImportUsersModal";
import type { ImportRow } from "../peopleImport";
import { FaTrash, FaPencilAlt } from "react-icons/fa";
import { FaUserLarge } from "react-icons/fa6";
import { Pagination, usePagination } from "../components/Pagination";

export function UsersSettings() {
  const { t } = useTranslation();
  const { arabicEnabled, lang, accounts } = useLayout();
  const primaryName = (name: string, ar?: string) => (lang === "ar" && ar ? ar : name);
  const secondaryName = (name: string, ar?: string) => (lang === "ar" ? name : ar);
  const [people, setPeople] = useState<Person[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [name, setName] = useState("");
  const [nameAr, setNameAr] = useState("");
  const [busy, setBusy] = useState(false);
  const [editingId, setEditingId] = useState<string | null>(null);
  const [editName, setEditName] = useState("");
  const [editNameAr, setEditNameAr] = useState("");
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [q, setQ] = useState("");
  const [showImport, setShowImport] = useState(false);

  // The accounts each person has a seat on. Seats store the person's name (the server keeps
  // them in sync when a person is renamed), so a seat belongs to the person whose name it
  // equals. An account counts once per person even if they hold several seats on it.
  const accountsOf = useMemo(() => {
    const m = new Map<string, AccountListItem[]>();
    for (const a of accounts) {
      const seen = new Set<string>();
      for (const seat of a.seats) {
        const name = seat.trim();
        if (!name || seen.has(name)) continue;
        seen.add(name);
        m.set(name, [...(m.get(name) ?? []), a]);
      }
    }
    for (const list of m.values()) list.sort((x, y) => x.service.localeCompare(y.service, "en", { sensitivity: "base" }));
    return m;
  }, [accounts]);
  // Which person's accounts popover is open (one at a time).
  const [openAccts, setOpenAccts] = useState<string | null>(null);

  // Search matches either the English or the Arabic name, regardless of the UI
  // language, so users are findable in both.
  const filtered = people.filter((p) => {
    const s = q.trim().toLowerCase();
    if (!s) return true;
    return p.name.toLowerCase().includes(s) || (p.name_ar ?? "").toLowerCase().includes(s);
  });

  async function refresh() {
    // A to Z by English name, ignoring case (so "khalid" sorts with the K's).
    try { setPeople((await listPeople()).sort((a, b) => a.name.localeCompare(b.name, "en", { sensitivity: "base" }))); } catch (e) { setError((e as Error).message); }
  }
  useEffect(() => { refresh(); }, []);

  function toggle(id: string) {
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id); else next.add(id);
      return next;
    });
  }
  const pager = usePagination(filtered, q.trim().toLowerCase());
  // "Select all" covers the rows on the current page only, so a bulk delete never
  // removes rows you can't see.
  const allSelected = pager.items.length > 0 && pager.items.every((p) => selected.has(p.id));
  function toggleAll() {
    setSelected((prev) => {
      const next = new Set(prev);
      for (const p of pager.items) { if (allSelected) next.delete(p.id); else next.add(p.id); }
      return next;
    });
  }

  async function bulkDelete() {
    const ids = [...selected];
    if (ids.length === 0) return;
    const ok = await confirmDialog({
      title: t("users.bulkTitle", { count: ids.length }),
      text: t("users.bulkText"),
      confirmText: t("common.delete"), danger: true,
    });
    if (!ok) return;
    try {
      const res = await deletePeople(ids);
      setSelected(new Set());
      await refresh();
      toast(t("users.deletedToast", { count: res.deleted }));
    } catch (e) { setError((e as Error).message); }
  }

  async function handleAdd(e: FormEvent) {
    e.preventDefault();
    setError(null);
    if (!name.trim()) return;
    setBusy(true);
    try { await addPerson(name.trim(), nameAr.trim() || undefined); setName(""); setNameAr(""); await refresh(); }
    catch (e) { setError((e as Error).message); }
    finally { setBusy(false); }
  }

  async function saveEdit(id: string) {
    try { await updatePerson(id, editName.trim(), editNameAr.trim() || undefined); setEditingId(null); await refresh(); }
    catch (e) { setError((e as Error).message); }
  }

  async function remove(p: Person) {
    const ok = await confirmDialog({
      title: t("users.deleteTitle", { name: p.name }),
      text: t("users.deleteText"),
      confirmText: t("common.delete"), danger: true,
    });
    if (!ok) return;
    try { await deletePerson(p.id); await refresh(); } catch (e) { setError((e as Error).message); }
  }

  // Parse an .xlsx/.csv: first column = name. When Arabic support is on, the
  // second column is taken as the Arabic name.
  async function runImport(rows: ImportRow[]) {
    const res = await importPeople(rows);
    setShowImport(false);
    await refresh();
    toast(t("users.imported", { count: res.added }));
  }

  return (
    <div className="card">
      <div className="topbar" style={{ marginBottom: 12, gap: 12 }}>
        <h2 className="section-title" style={{ margin: 0 }}>{t("users.title")}</h2>
        <Button onClick={() => setShowImport(true)}>{t("users.importExcel")}</Button>
      </div>
      {error && <p className="error">{error}</p>}

      <div className="add-user-row">
      <h2 className="section-title" style={{ margin: 0 }}>{t("users.add")}</h2>
      <form className="inline-add" onSubmit={handleAdd}>
        <input value={name} onChange={(e) => setName(e.target.value)} placeholder={t("users.namePlaceholder")} required />
        {arabicEnabled && (
          <input value={nameAr} onChange={(e) => setNameAr(e.target.value)} dir="rtl" placeholder={t("common.namePlaceholderAr")} />
        )}
        <CircleButton type="submit" disabled={busy} title={t("users.add")}>+</CircleButton>
      </form>
      </div>

      {people.length > 0 && (
        <div className="bulk-bar">
          <label className="bulk-select">
            <input type="checkbox" checked={allSelected} onChange={toggleAll} />
            <span>{t("users.selectAll")}</span>
          </label>
          {selected.size > 0 && (
            <div className="bulk-actions">
              <span className="muted">{t("users.selectedCount", { count: selected.size })}</span>
              <Button variant="danger" onClick={bulkDelete}><FaTrash /> {t("users.deleteSelected")}</Button>
            </div>
          )}
          <input className="user-search" value={q} onChange={(e) => setQ(e.target.value)}
            placeholder={t("users.search")} />
        </div>
      )}

      <ul className="cat-list">
        {pager.items.map((p) => (
          <li key={p.id} className={selected.has(p.id) ? "row-selected" : ""}>
            <span className="user-left">
              <input type="checkbox" className="row-check" checked={selected.has(p.id)} onChange={() => toggle(p.id)} />
              <span>
                <strong>{primaryName(p.name, p.name_ar)}</strong>{arabicEnabled && secondaryName(p.name, p.name_ar) && <span className="muted"> · {secondaryName(p.name, p.name_ar)}</span>}
              </span>
            </span>
            <span className="att-actions">
              <UserAccountsMenu
                list={accountsOf.get(p.name.trim()) ?? []}
                open={openAccts === p.id}
                onToggle={() => setOpenAccts((cur) => (cur === p.id ? null : p.id))}
                onClose={() => setOpenAccts(null)}
              />
              <IconButton title={t("common.edit")} onClick={() => { setEditingId(p.id); setEditName(p.name); setEditNameAr(p.name_ar ?? ""); }}><FaPencilAlt /></IconButton>
              <IconButton danger title={t("common.delete")} onClick={() => remove(p)}><FaTrash /></IconButton>
            </span>
          </li>
        ))}
        {people.length === 0 && <li className="muted">{t("users.none")}</li>}
        {people.length > 0 && filtered.length === 0 && <li className="muted">{t("users.noMatch")}</li>}
      </ul>
      <Pagination p={pager} />

      {showImport && (
        <ImportUsersModal arabicEnabled={arabicEnabled} onImport={runImport} onClose={() => setShowImport(false)} />
      )}

      {editingId && (
        <Modal onClose={() => setEditingId(null)}>
          <h2 className="section-title">{t("users.editTitle")}</h2>
          <div className="add-form">
            <Field label={t("common.name")}>
              <input value={editName} onChange={(e) => setEditName(e.target.value)} autoFocus />
            </Field>
            {arabicEnabled && (
              <Field label={t("common.nameArabic")}>
                <input value={editNameAr} onChange={(e) => setEditNameAr(e.target.value)} dir="rtl" placeholder={t("common.namePlaceholderAr")} />
              </Field>
            )}
            <div className="form-actions">
              <Button onClick={() => saveEdit(editingId)}>{t("common.save")}</Button>
              <Button variant="ghost" onClick={() => setEditingId(null)}>{t("common.cancel")}</Button>
            </div>
          </div>
        </Modal>
      )}
    </div>
  );
}

// A user icon (FaUserLarge) next to the pencil: hover it (or focus it with the keyboard) to see the accounts
// this person has a seat on, in a small popover; each row opens that account. On a touch
// screen, tap the icon instead; the popover then closes on a tap elsewhere or Escape.
function UserAccountsMenu({ list, open, onToggle, onClose }: { list: AccountListItem[]; open: boolean; onToggle: () => void; onClose: () => void }) {
  const { t } = useTranslation();
  const ref = useRef<HTMLSpanElement>(null);

  useEffect(() => {
    if (!open) return;
    const onDown = (e: MouseEvent) => { if (!ref.current?.contains(e.target as Node)) onClose(); };
    const onKey = (e: KeyboardEvent) => { if (e.key === "Escape") onClose(); };
    document.addEventListener("mousedown", onDown);
    document.addEventListener("keydown", onKey);
    return () => { document.removeEventListener("mousedown", onDown); document.removeEventListener("keydown", onKey); };
  }, [open, onClose]);

  return (
    <span className="acct-pop-wrap" ref={ref}>
      <IconButton title={t("home.accountsCount", { count: list.length })} onClick={onToggle}><FaUserLarge /></IconButton>
      {/* Always rendered: CSS shows it on hover or focus; `open` shows it after a tap. */}
      <div className={`acct-pop ${open ? "open" : ""}`.trim()} role="menu">
        <div className="acct-pop-box">
          {list.length === 0 ? (
            <span className="acct-pop-empty muted">{t("users.noAccounts")}</span>
          ) : (
            list.map((a) => (
              <Link key={a.id} to={`/account/${a.id}`} className="acct-pop-row" role="menuitem">
                <i className="dot dot-account" />
                <span>{a.service}</span>
                {a.login && <span className="muted acct-pop-login">{a.login}</span>}
              </Link>
            ))
          )}
        </div>
      </div>
    </span>
  );
}
