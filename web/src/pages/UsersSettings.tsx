import { useEffect, useRef, useState, type FormEvent } from "react";
import { useTranslation } from "react-i18next";
import * as XLSX from "xlsx";
import { listPeople, addPerson, updatePerson, deletePerson, deletePeople, importPeople, type Person } from "../api";
import { confirmDialog, toast } from "../alerts";
import { useLayout } from "../components/Layout";
import { Button } from "../components/Button";
import { IconButton } from "../components/IconButton";
import { CircleButton } from "../components/CircleButton";
import { Field } from "../components/Field";
import { Modal } from "../components/Modal";
import { FaTrash, FaPencilAlt } from "react-icons/fa";
import { Pagination, usePagination } from "../components/Pagination";

export function UsersSettings() {
  const { t } = useTranslation();
  const { arabicEnabled, lang } = useLayout();
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
  const fileRef = useRef<HTMLInputElement>(null);

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
  async function handleImport(e: React.ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0];
    if (!file) return;
    setError(null);
    try {
      const buf = await file.arrayBuffer();
      const wb = XLSX.read(buf);
      const sheet = wb.Sheets[wb.SheetNames[0]];
      const rows = XLSX.utils.sheet_to_json<string[]>(sheet, { header: 1 });
      const people = rows
        .map((r) => {
          if (!Array.isArray(r)) return null;
          const name = String(r[0] ?? "").trim();
          if (!name) return null;
          const nameAr = arabicEnabled ? String(r[1] ?? "").trim() : "";
          return { name, name_ar: nameAr || undefined };
        })
        .filter((p): p is { name: string; name_ar: string | undefined } => p !== null);
      if (people.length === 0) { setError(t("users.noNamesFound")); return; }
      const res = await importPeople(people);
      await refresh();
      toast(t("users.imported", { count: res.added }));
    } catch (err) {
      setError((err as Error).message);
    } finally {
      if (fileRef.current) fileRef.current.value = "";
    }
  }

  return (
    <div className="card">
      <div className="topbar" style={{ marginBottom: 12, gap: 12 }}>
        <h2 className="section-title" style={{ margin: 0 }}>{t("users.title")}</h2>
        <label className="btn-like" title={arabicEnabled ? t("users.importHintAr") : t("users.importHint")}>
          {t("users.importExcel")}
          <input ref={fileRef} type="file" accept=".xlsx,.xls,.csv" hidden onChange={handleImport} />
        </label>
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
              <span><strong>{primaryName(p.name, p.name_ar)}</strong>{arabicEnabled && secondaryName(p.name, p.name_ar) && <span className="muted"> · {secondaryName(p.name, p.name_ar)}</span>}</span>
            </span>
            <span className="att-actions">
              <IconButton title={t("common.edit")} onClick={() => { setEditingId(p.id); setEditName(p.name); setEditNameAr(p.name_ar ?? ""); }}><FaPencilAlt /></IconButton>
              <IconButton danger title={t("common.delete")} onClick={() => remove(p)}><FaTrash /></IconButton>
            </span>
          </li>
        ))}
        {people.length === 0 && <li className="muted">{t("users.none")}</li>}
        {people.length > 0 && filtered.length === 0 && <li className="muted">{t("users.noMatch")}</li>}
      </ul>
      <Pagination p={pager} />

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
