import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { useLayout } from "../components/Layout";
import { createCategory, updateCategory, deleteCategory, reorderCategories, type Category } from "../api";
import { Icon, sanitizeIconName } from "../icons";
import { confirmDialog } from "../alerts";
import { FaPencilAlt, FaTrash, FaGripVertical } from "react-icons/fa";
import { Button } from "../components/Button";
import { IconButton } from "../components/IconButton";
import { CircleButton } from "../components/CircleButton";
import { Field } from "../components/Field";
import { Modal } from "../components/Modal";

export function CategoriesSettings() {
  const { t } = useTranslation();
  const { categories, refreshCategories, refresh, arabicEnabled } = useLayout();
  const [error, setError] = useState<string | null>(null);
  const [editingId, setEditingId] = useState<string | null>(null);
  const [name, setName] = useState("");
  const [nameAr, setNameAr] = useState("");
  const [icon, setIcon] = useState("FaFolder");

  // Local ordered copy for drag-and-drop; kept in sync with the shared list.
  const [items, setItems] = useState<Category[]>(categories);
  useEffect(() => { setItems(categories); }, [categories]);
  const dragIndex = useRef<number | null>(null);
  const [dragging, setDragging] = useState<number | null>(null);
  const [overIndex, setOverIndex] = useState<number | null>(null);

  function onDrop(toIndex: number) {
    const from = dragIndex.current;
    dragIndex.current = null;
    setDragging(null);
    setOverIndex(null);
    if (from === null || from === toIndex) return;
    const next = [...items];
    const [moved] = next.splice(from, 1);
    next.splice(toIndex, 0, moved);
    setItems(next);
    reorderCategories(next.map((c) => c.id)).then(refreshCategories).catch((e) => setError((e as Error).message));
  }

  function startAdd() { setEditingId("new"); setName(""); setNameAr(""); setIcon("FaFolder"); setError(null); }
  function startEdit(c: Category) { setEditingId(c.id); setName(c.name); setNameAr(c.name_ar ?? ""); setIcon(c.icon); setError(null); }

  async function save() {
    if (!name.trim()) { setError(t("categories.nameRequired")); return; }
    const iconName = sanitizeIconName(icon) || "FaFolder";
    const ar = nameAr.trim() || undefined;
    try {
      if (editingId === "new") await createCategory(name.trim(), iconName, ar);
      else if (editingId) await updateCategory(editingId, name.trim(), iconName, ar);
      setEditingId(null);
      refreshCategories();
    } catch (err) { setError((err as Error).message); }
  }

  async function remove(c: Category) {
    const ok = await confirmDialog({
      title: t("categories.deleteTitle", { name: c.name }),
      text: t("categories.deleteText"),
      confirmText: t("common.delete"),
      danger: true,
    });
    if (!ok) return;
    try { await deleteCategory(c.id); refreshCategories(); refresh(); }
    catch (err) { setError((err as Error).message); }
  }

  return (
    <div className="card">
      <div className="topbar" style={{ marginBottom: 12 }}>
        <h2 className="section-title" style={{ margin: 0 }}>{t("categories.title")}</h2>
        <CircleButton onClick={startAdd} title={t("categories.new")}>+</CircleButton>
      </div>
      {error && <p className="error">{error}</p>}

      <ul className="cat-list">
        {items.map((c, i) => (
          <li
            key={c.id}
            className={`${dragging === i ? "dragging" : ""} ${overIndex === i && dragging !== i ? "drag-over" : ""}`.trim()}
            draggable
            onDragStart={() => { dragIndex.current = i; setDragging(i); }}
            onDragEnd={() => { setDragging(null); setOverIndex(null); }}
            onDragOver={(e) => { e.preventDefault(); if (overIndex !== i) setOverIndex(i); }}
            onDrop={() => onDrop(i)}
          >
            <span className="cat-name">
              <span className="drag-handle" title={t("categories.dragToReorder")}>
                <FaGripVertical />
              </span>
              <Icon name={c.icon} /> {c.name}
              {arabicEnabled && c.name_ar && <span className="muted"> · {c.name_ar}</span>}
            </span>
            <span className="att-actions">
              <IconButton title={t("common.edit")} onClick={() => startEdit(c)}><FaPencilAlt /></IconButton>
              <IconButton danger title={t("common.delete")} onClick={() => remove(c)}><FaTrash /></IconButton>
            </span>
          </li>
        ))}
        {items.length === 0 && <li className="muted">{t("categories.none")}</li>}
      </ul>

      {editingId && (
        <Modal onClose={() => setEditingId(null)}>
          <h2 className="section-title">{editingId === "new" ? t("categories.new") : t("categories.editTitle")}</h2>
          <div className="add-form">
            <Field label={t("common.name")}>
              <input value={name} onChange={(e) => setName(e.target.value)} autoFocus />
            </Field>
            {arabicEnabled && (
              <Field label={t("common.nameArabic")}>
                <input value={nameAr} onChange={(e) => setNameAr(e.target.value)} dir="rtl" placeholder={t("common.namePlaceholderAr")} />
              </Field>
            )}
            <Field
              label={t("categories.iconName")}
              hint={<>{t("categories.iconHintPre")} <a href="https://react-icons.github.io/react-icons/" target="_blank" rel="noreferrer">{t("categories.iconHintLink")}</a>.</>}
            >
              <span className="icon-input">
                <span className="icon-preview"><Icon name={sanitizeIconName(icon)} size={22} /></span>
                <input value={icon} onChange={(e) => setIcon(e.target.value)} placeholder={t("categories.iconPlaceholder")} />
              </span>
            </Field>
            {error && <p className="error">{error}</p>}
            <div className="form-actions">
              <Button onClick={save}>{editingId === "new" ? t("categories.addBtn") : t("common.save")}</Button>
              <Button variant="ghost" onClick={() => setEditingId(null)}>{t("common.cancel")}</Button>
            </div>
          </div>
        </Modal>
      )}
    </div>
  );
}
