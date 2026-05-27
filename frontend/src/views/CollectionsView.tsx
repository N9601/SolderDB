import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  CreateCollection,
  DeleteCollection,
  DeleteRecord,
  InsertRecord,
  ListCollections,
  ListRecords,
  UpdateCollection,
  UpdateRecord
} from "../wailsjs/go/bridge/CollectionsService";
import { GetAPIAddr } from "../wailsjs/go/bridge/DBService";
import { bridge } from "../wailsjs/go/models";
import { withAuthQuery } from "../lib/apiFetch";

type Collection = bridge.CollectionMeta;
type Field = bridge.Field;
type Record_ = bridge.Document;

type FieldType = "text" | "number" | "bool" | "json" | "date";
const FIELD_TYPES: FieldType[] = ["text", "number", "bool", "json", "date"];

type Props = {
  onStatus: (s: string) => void;
};

export default function CollectionsView({ onStatus }: Props) {
  const [collections, setCollections] = useState<Collection[]>([]);
  const [selected, setSelected] = useState<Collection | null>(null);
  const [records, setRecords] = useState<Record_[]>([]);
  const [creatingCollection, setCreatingCollection] = useState(false);
  const [editingSchema, setEditingSchema] = useState(false);
  const [editingRecord, setEditingRecord] = useState<Record_ | null>(null);
  const [insertOpen, setInsertOpen] = useState(false);
  const [livePulse, setLivePulse] = useState<string>("");
  const livePulseTimer = useRef<number | null>(null);

  const refresh = useCallback(async () => {
    try {
      const list = await ListCollections();
      setCollections(list ?? []);
      if (selected && !(list ?? []).some((c) => c.name === selected.name)) {
        setSelected(null);
        setRecords([]);
      }
    } catch (e) {
      onStatus(`Collections error: ${String(e)}`);
    }
  }, [onStatus, selected]);

  const refreshRecords = useCallback(async () => {
    if (!selected) {
      setRecords([]);
      return;
    }
    try {
      const res = await ListRecords(selected.name, "", 100);
      setRecords(res.records ?? []);
    } catch (e) {
      onStatus(`List records error: ${String(e)}`);
    }
  }, [selected, onStatus]);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  useEffect(() => {
    void refreshRecords();
  }, [refreshRecords]);

  // Live: subscribe to changes on the selected collection via SSE.
  useEffect(() => {
    if (!selected) return;
    let es: EventSource | null = null;
    let cancelled = false;
    void (async () => {
      try {
        const apiAddr = await GetAPIAddr();
        if (!apiAddr || cancelled) return;
        es = new EventSource(withAuthQuery(`${apiAddr}/api/realtime?topic=coll:${encodeURIComponent(selected.name)}`));
        const onEvent = (kind: string) => {
          setLivePulse(kind);
          if (livePulseTimer.current !== null) window.clearTimeout(livePulseTimer.current);
          livePulseTimer.current = window.setTimeout(() => setLivePulse(""), 1200);
          void refreshRecords();
        };
        es.addEventListener("create", () => onEvent("create"));
        es.addEventListener("update", () => onEvent("update"));
        es.addEventListener("delete", () => onEvent("delete"));
      } catch {
        // ignore — UI keeps working without realtime
      }
    })();
    return () => {
      cancelled = true;
      if (es) es.close();
      if (livePulseTimer.current !== null) window.clearTimeout(livePulseTimer.current);
    };
  }, [selected, refreshRecords]);

  async function onDeleteCollection(name: string) {
    if (!window.confirm(`Delete collection "${name}" and all its records?`)) return;
    try {
      await DeleteCollection(name);
      onStatus(`Deleted collection ${name}`);
      if (selected?.name === name) setSelected(null);
      await refresh();
    } catch (e) {
      onStatus(`Delete error: ${String(e)}`);
    }
  }

  async function onDeleteRecord(id: string) {
    if (!selected) return;
    if (!window.confirm(`Delete record ${id}?`)) return;
    try {
      await DeleteRecord(selected.name, id);
      onStatus(`Deleted record ${id}`);
      await refreshRecords();
    } catch (e) {
      onStatus(`Delete error: ${String(e)}`);
    }
  }

  return (
    <div className="animate-slideUp grid grid-cols-12 gap-5">
      {/* Left: collections list */}
      <aside className="col-span-3">
        <div className="card overflow-hidden">
          <div className="flex items-center justify-between border-b border-canvas-200 px-4 py-3">
            <div className="section-title">Collections</div>
            <button className="btn btn-ghost btn-icon" onClick={() => setCreatingCollection(true)} title="New collection">
              <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                <line x1="12" y1="5" x2="12" y2="19" />
                <line x1="5" y1="12" x2="19" y2="12" />
              </svg>
            </button>
          </div>
          <div className="max-h-[60vh] overflow-auto">
            {collections.length === 0 ? (
              <div className="px-4 py-6 text-center text-[12px] text-ink-400">
                No collections yet.
                <div className="mt-2">
                  <button className="btn btn-primary" onClick={() => setCreatingCollection(true)}>
                    Create your first
                  </button>
                </div>
              </div>
            ) : (
              collections.map((c) => (
                <button
                  key={c.name}
                  onClick={() => setSelected(c)}
                  className={`flex w-full items-center justify-between border-b border-canvas-150 px-4 py-2.5 text-left text-[13px] transition-colors hover:bg-canvas-100 ${
                    selected?.name === c.name ? "bg-copper-50" : ""
                  }`}
                >
                  <div>
                    <div className="font-medium text-ink-900">{c.name}</div>
                    <div className="text-[11px] text-ink-400">
                      {c.fields.length} field{c.fields.length === 1 ? "" : "s"}
                    </div>
                  </div>
                  <span
                    className="text-ink-300 opacity-0 transition-opacity hover:text-danger group-hover:opacity-100"
                    onClick={(e) => {
                      e.stopPropagation();
                      void onDeleteCollection(c.name);
                    }}
                  >
                    ✕
                  </span>
                </button>
              ))
            )}
          </div>
        </div>
      </aside>

      {/* Right: records of selected collection */}
      <section className="col-span-9 space-y-4">
        {!selected ? (
          <EmptyHero onCreate={() => setCreatingCollection(true)} />
        ) : (
          <>
            <CollectionHeader
              collection={selected}
              livePulse={livePulse}
              onInsert={() => setInsertOpen(true)}
              onEditSchema={() => setEditingSchema(true)}
              onDelete={() => void onDeleteCollection(selected.name)}
            />
            <RecordsTable
              collection={selected}
              records={records}
              onEdit={(r) => setEditingRecord(r)}
              onDelete={(id) => void onDeleteRecord(id)}
            />
          </>
        )}
      </section>

      {/* Modals */}
      {creatingCollection && (
        <CreateCollectionModal
          onClose={() => setCreatingCollection(false)}
          onCreated={async (c) => {
            setCreatingCollection(false);
            onStatus(`Created collection ${c.name}`);
            await refresh();
            setSelected(c);
          }}
          onError={(e) => onStatus(`Create error: ${e}`)}
        />
      )}
      {insertOpen && selected && (
        <RecordModal
          collection={selected}
          initial={null}
          onClose={() => setInsertOpen(false)}
          onSubmit={async (data) => {
            try {
              const r = await InsertRecord(selected.name, data);
              onStatus(`Inserted ${r.id}`);
              setInsertOpen(false);
              await refreshRecords();
            } catch (e) {
              onStatus(`Insert error: ${String(e)}`);
            }
          }}
        />
      )}
      {editingSchema && selected && (
        <SchemaEditorModal
          collection={selected}
          onClose={() => setEditingSchema(false)}
          onSaved={async (c) => {
            setEditingSchema(false);
            onStatus(`Updated schema for ${c.name}`);
            setSelected(c);
            await refresh();
          }}
          onError={(e) => onStatus(`Schema error: ${e}`)}
        />
      )}
      {editingRecord && selected && (
        <RecordModal
          collection={selected}
          initial={editingRecord}
          onClose={() => setEditingRecord(null)}
          onSubmit={async (data) => {
            try {
              await UpdateRecord(selected.name, editingRecord.id, data);
              onStatus(`Updated ${editingRecord.id}`);
              setEditingRecord(null);
              await refreshRecords();
            } catch (e) {
              onStatus(`Update error: ${String(e)}`);
            }
          }}
        />
      )}
    </div>
  );
}

/* ---------- Pieces ---------- */

function EmptyHero({ onCreate }: { onCreate: () => void }) {
  return (
    <div className="card card-pad flex flex-col items-center py-16">
      <div className="text-[15px] font-semibold text-ink-900">Pick a collection</div>
      <div className="mt-1 text-[12px] text-ink-400">
        Collections give your KV store typed records, schemas, and validation.
      </div>
      <button className="btn btn-primary mt-5" onClick={onCreate}>
        + New collection
      </button>
    </div>
  );
}

function CollectionHeader(props: {
  collection: Collection;
  livePulse: string;
  onInsert: () => void;
  onEditSchema: () => void;
  onDelete: () => void;
}) {
  const { collection } = props;
  return (
    <div className="card card-pad">
      <div className="flex items-start justify-between gap-4">
        <div>
          <div className="flex items-center gap-2">
            <h2 className="text-[16px] font-semibold text-ink-900">{collection.name}</h2>
            <span className="chip chip-mono">{collection.fields.length} fields</span>
            <span
              className={`chip chip-mono ${props.livePulse ? "chip-copper animate-pulseCopper" : ""}`}
              title="Live updates via Server-Sent Events"
            >
              <span className={`dot ${props.livePulse ? "" : "dot-idle"}`} />
              {props.livePulse ? `LIVE · ${props.livePulse.toUpperCase()}` : "LIVE"}
            </span>
          </div>
          <div className="mt-1 text-[11px] text-ink-400">
            Created {fmtDate(collection.created)} · Updated {fmtDate(collection.updated)}
          </div>
          <div className="mt-3 flex flex-wrap gap-1.5">
            {collection.fields.map((f) => (
              <span key={f.name} className={`chip chip-mono ${typeChipCls(f.type as FieldType)}`}>
                {f.name}
                <span className="opacity-60">:{f.type}</span>
                {f.required && <span className="text-copper-600">*</span>}
              </span>
            ))}
          </div>
          <div className="mt-2 flex flex-wrap gap-1.5 text-[10.5px]">
            <RuleBadge op="list" rule={collection.listRule ?? ""} />
            <RuleBadge op="view" rule={collection.viewRule ?? ""} />
            <RuleBadge op="create" rule={collection.createRule ?? ""} />
            <RuleBadge op="update" rule={collection.updateRule ?? ""} />
            <RuleBadge op="delete" rule={collection.deleteRule ?? ""} />
          </div>
        </div>
        <div className="flex gap-2">
          <button className="btn" onClick={props.onEditSchema}>
            Edit schema
          </button>
          <button className="btn btn-danger" onClick={props.onDelete}>
            Delete
          </button>
          <button className="btn btn-primary" onClick={props.onInsert}>
            + New record
          </button>
        </div>
      </div>
    </div>
  );
}

function RecordsTable(props: {
  collection: Collection;
  records: Record_[];
  onEdit: (r: Record_) => void;
  onDelete: (id: string) => void;
}) {
  const { collection, records } = props;
  const columns = useMemo(() => collection.fields.slice(0, 4), [collection.fields]);

  if (records.length === 0) {
    return (
      <div className="card card-pad py-12 text-center text-[12px] text-ink-400">
        No records yet — click <strong>+ New record</strong>.
      </div>
    );
  }

  return (
    <div className="card overflow-hidden">
      <div className="overflow-x-auto">
        <table className="w-full text-left text-[12.5px]">
          <thead>
            <tr className="border-b border-canvas-200 bg-canvas-100 text-[10.5px] uppercase tracking-wider text-ink-400">
              <th className="px-4 py-2.5 font-semibold">ID</th>
              {columns.map((c) => (
                <th key={c.name} className="px-4 py-2.5 font-semibold">
                  {c.name}
                </th>
              ))}
              <th className="px-4 py-2.5 font-semibold">Updated</th>
              <th className="px-2 py-2.5"></th>
            </tr>
          </thead>
          <tbody>
            {records.map((r) => (
              <tr
                key={r.id}
                className="border-b border-canvas-150 transition-colors hover:bg-canvas-100"
                onClick={() => props.onEdit(r)}
              >
                <td className="cursor-pointer px-4 py-2.5 font-mono text-[11.5px] text-ink-500">
                  {r.id.slice(0, 10)}…
                </td>
                {columns.map((c) => (
                  <td key={c.name} className="cursor-pointer px-4 py-2.5 font-mono text-ink-900">
                    {renderCell(r.data[c.name], c.type as FieldType)}
                  </td>
                ))}
                <td className="cursor-pointer px-4 py-2.5 text-[11px] text-ink-400">{fmtDate(r.updated)}</td>
                <td className="px-2 py-2.5 text-right">
                  <button
                    className="btn btn-ghost btn-icon"
                    onClick={(e) => {
                      e.stopPropagation();
                      props.onDelete(r.id);
                    }}
                    title="Delete"
                  >
                    ✕
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}

function CreateCollectionModal(props: {
  onClose: () => void;
  onCreated: (c: Collection) => void;
  onError: (e: string) => void;
}) {
  const [name, setName] = useState("");
  const [fields, setFields] = useState<Field[]>([
    { name: "title", type: "text", required: true, unique: false }
  ]);

  function updateField(i: number, patch: Partial<Field>) {
    setFields(fields.map((f, idx) => (idx === i ? { ...f, ...patch } : f)));
  }
  function addField() {
    setFields([...fields, { name: "", type: "text", required: false, unique: false }]);
  }
  function removeField(i: number) {
    setFields(fields.filter((_, idx) => idx !== i));
  }

  async function submit() {
    try {
      const cleaned = fields.filter((f) => f.name.trim() !== "");
      const created = await CreateCollection(
        bridge.CollectionMeta.createFrom({
          name: name.trim(),
          fields: cleaned,
          created: "",
          updated: ""
        })
      );
      props.onCreated(created);
    } catch (e) {
      props.onError(String(e));
    }
  }

  return (
    <Modal onClose={props.onClose} title="New collection" wide>
      <div className="space-y-4">
        <div>
          <label className="label">Name</label>
          <input
            value={name}
            onChange={(e) => setName(e.target.value)}
            className="field mt-1"
            placeholder="users, posts, parts…"
            spellCheck={false}
          />
          <div className="mt-1 text-[11px] text-ink-400">
            Lowercase letters, digits, underscores. Starts with a letter.
          </div>
        </div>

        <div>
          <div className="flex items-center justify-between">
            <label className="label">Fields</label>
            <button className="btn btn-ghost" onClick={addField}>
              + Field
            </button>
          </div>
          <div className="mt-2 space-y-2">
            {fields.map((f, i) => (
              <div key={i} className="grid grid-cols-12 items-center gap-2 rounded-md border border-canvas-200 bg-canvas-100 p-2">
                <input
                  value={f.name}
                  onChange={(e) => updateField(i, { name: e.target.value })}
                  placeholder="field_name"
                  className="field col-span-5"
                />
                <select
                  value={f.type}
                  onChange={(e) => updateField(i, { type: e.target.value })}
                  className="field col-span-3"
                >
                  {FIELD_TYPES.map((t) => (
                    <option key={t} value={t}>
                      {t}
                    </option>
                  ))}
                </select>
                <label className="col-span-3 flex items-center gap-2 text-[12px] text-ink-700">
                  <input
                    type="checkbox"
                    checked={f.required}
                    onChange={(e) => updateField(i, { required: e.target.checked })}
                  />
                  Required
                </label>
                <button className="btn btn-ghost btn-icon col-span-1" onClick={() => removeField(i)} title="Remove">
                  ✕
                </button>
              </div>
            ))}
          </div>
        </div>

        <div className="flex justify-end gap-2 border-t border-canvas-200 pt-4">
          <button className="btn" onClick={props.onClose}>
            Cancel
          </button>
          <button className="btn btn-primary" onClick={() => void submit()} disabled={!name.trim()}>
            Create
          </button>
        </div>
      </div>
    </Modal>
  );
}

type RuleName = "public" | "authed" | "admin";
const RULES: RuleName[] = ["public", "authed", "admin"];

function SchemaEditorModal(props: {
  collection: Collection;
  onClose: () => void;
  onSaved: (c: Collection) => void;
  onError: (e: string) => void;
}) {
  const [fields, setFields] = useState<Field[]>(() => props.collection.fields.map((f) => ({ ...f })));
  const [rules, setRules] = useState<{ list: RuleName; view: RuleName; create: RuleName; update: RuleName; delete: RuleName }>({
    list: ((props.collection.listRule || "authed") as RuleName),
    view: ((props.collection.viewRule || "authed") as RuleName),
    create: ((props.collection.createRule || "authed") as RuleName),
    update: ((props.collection.updateRule || "authed") as RuleName),
    delete: ((props.collection.deleteRule || "authed") as RuleName)
  });

  function updateField(i: number, patch: Partial<Field>) {
    setFields(fields.map((f, idx) => (idx === i ? { ...f, ...patch } : f)));
  }
  function addField() {
    setFields([...fields, { name: "", type: "text", required: false, unique: false }]);
  }
  function removeField(i: number) {
    setFields(fields.filter((_, idx) => idx !== i));
  }

  async function submit() {
    try {
      const cleaned = fields.filter((f) => f.name.trim() !== "");
      const updated = await UpdateCollection(
        props.collection.name,
        bridge.UpdatePatch.createFrom({
          fields: cleaned,
          listRule: rules.list,
          viewRule: rules.view,
          createRule: rules.create,
          updateRule: rules.update,
          deleteRule: rules.delete
        })
      );
      props.onSaved(updated);
    } catch (e) {
      props.onError(String(e));
    }
  }

  return (
    <Modal onClose={props.onClose} title={`Edit schema · ${props.collection.name}`} wide>
      <div className="space-y-5">
        <div className="rounded-md border border-amber-200 bg-amber-50 px-3 py-2 text-[11.5px] text-amber-800">
          Removing a field doesn&apos;t delete data from existing records — old values remain on disk but become invisible.
          Renaming is a remove + add.
        </div>

        <div>
          <div className="flex items-center justify-between">
            <label className="label">Fields</label>
            <button className="btn btn-ghost" onClick={addField}>
              + Field
            </button>
          </div>
          <div className="mt-2 space-y-2">
            {fields.map((f, i) => (
              <div key={i} className="grid grid-cols-12 items-center gap-2 rounded-md border border-canvas-200 bg-canvas-100 p-2">
                <input
                  value={f.name}
                  onChange={(e) => updateField(i, { name: e.target.value })}
                  placeholder="field_name"
                  className="field col-span-5"
                />
                <select
                  value={f.type}
                  onChange={(e) => updateField(i, { type: e.target.value })}
                  className="field col-span-3"
                >
                  {FIELD_TYPES.map((t) => (
                    <option key={t} value={t}>
                      {t}
                    </option>
                  ))}
                </select>
                <label className="col-span-3 flex items-center gap-2 text-[12px] text-ink-700">
                  <input
                    type="checkbox"
                    checked={f.required}
                    onChange={(e) => updateField(i, { required: e.target.checked })}
                  />
                  Required
                </label>
                <button className="btn btn-ghost btn-icon col-span-1" onClick={() => removeField(i)} title="Remove">
                  ✕
                </button>
              </div>
            ))}
          </div>
        </div>

        <div>
          <label className="label">Access rules</label>
          <div className="mt-1 text-[11px] text-ink-400">
            <span className="font-semibold text-copper-700">public</span> — anyone ·{" "}
            <span className="font-semibold text-steel-700">authed</span> — any logged-in user ·{" "}
            <span className="font-semibold text-ink-700">admin</span> — admins only
          </div>
          <div className="mt-2 grid grid-cols-1 gap-2 md:grid-cols-5">
            {(["list", "view", "create", "update", "delete"] as const).map((op) => (
              <div key={op}>
                <div className="text-[11px] font-medium uppercase tracking-wider text-ink-500">{op}</div>
                <select
                  value={rules[op]}
                  onChange={(e) => setRules({ ...rules, [op]: e.target.value as RuleName })}
                  className="field mt-1"
                >
                  {RULES.map((r) => (
                    <option key={r} value={r}>
                      {r}
                    </option>
                  ))}
                </select>
              </div>
            ))}
          </div>
        </div>

        <div className="flex justify-end gap-2 border-t border-canvas-200 pt-4">
          <button className="btn" onClick={props.onClose}>
            Cancel
          </button>
          <button className="btn btn-primary" onClick={() => void submit()} disabled={fields.length === 0}>
            Save schema
          </button>
        </div>
      </div>
    </Modal>
  );
}

function RecordModal(props: {
  collection: Collection;
  initial: Record_ | null;
  onClose: () => void;
  onSubmit: (data: { [k: string]: unknown }) => Promise<void>;
}) {
  const [data, setData] = useState<{ [k: string]: unknown }>(() => {
    if (props.initial) return { ...props.initial.data };
    const out: { [k: string]: unknown } = {};
    for (const f of props.collection.fields) {
      out[f.name] = f.type === "bool" ? false : "";
    }
    return out;
  });
  const [submitting, setSubmitting] = useState(false);

  async function submit() {
    setSubmitting(true);
    const coerced: { [k: string]: unknown } = {};
    for (const f of props.collection.fields) {
      const raw = data[f.name];
      coerced[f.name] = coerceForType(raw, f.type as FieldType);
    }
    try {
      await props.onSubmit(coerced);
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <Modal onClose={props.onClose} title={props.initial ? `Edit ${props.initial.id.slice(0, 12)}…` : "New record"} wide>
      <div className="space-y-3">
        {props.collection.fields.map((f) => (
          <FieldInput
            key={f.name}
            field={f}
            value={data[f.name]}
            onChange={(v) => setData({ ...data, [f.name]: v })}
          />
        ))}
        <div className="flex justify-end gap-2 border-t border-canvas-200 pt-4">
          <button className="btn" onClick={props.onClose}>
            Cancel
          </button>
          <button className="btn btn-primary" onClick={() => void submit()} disabled={submitting}>
            {props.initial ? "Save" : "Insert"}
          </button>
        </div>
      </div>
    </Modal>
  );
}

function FieldInput(props: { field: Field; value: unknown; onChange: (v: unknown) => void }) {
  const { field, value } = props;
  const t = field.type as FieldType;
  return (
    <div>
      <label className="label">
        {field.name}
        <span className="ml-2 text-ink-300">{field.type}</span>
        {field.required && <span className="ml-1 text-copper-600">*</span>}
      </label>
      {t === "bool" ? (
        <div className="mt-1">
          <input
            type="checkbox"
            checked={Boolean(value)}
            onChange={(e) => props.onChange(e.target.checked)}
          />
        </div>
      ) : t === "json" ? (
        <textarea
          value={typeof value === "string" ? value : JSON.stringify(value, null, 2)}
          onChange={(e) => props.onChange(e.target.value)}
          className="field mt-1"
          rows={4}
          spellCheck={false}
          placeholder='{"key": "value"}'
        />
      ) : (
        <input
          value={value == null ? "" : String(value)}
          onChange={(e) => props.onChange(e.target.value)}
          className="field mt-1"
          type={t === "number" ? "number" : t === "date" ? "datetime-local" : "text"}
          spellCheck={false}
        />
      )}
    </div>
  );
}

function Modal(props: { title: string; onClose: () => void; wide?: boolean; children: React.ReactNode }) {
  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-ink-900/40 backdrop-blur-sm"
      onClick={props.onClose}
    >
      <div
        className={`animate-slideUp w-full ${props.wide ? "max-w-2xl" : "max-w-md"} rounded-xl border border-canvas-200 bg-white shadow-cardHover`}
        onClick={(e) => e.stopPropagation()}
      >
        <div className="flex items-center justify-between border-b border-canvas-200 px-5 py-3">
          <div className="text-[14px] font-semibold text-ink-900">{props.title}</div>
          <button className="btn btn-ghost btn-icon" onClick={props.onClose}>
            ✕
          </button>
        </div>
        <div className="p-5">{props.children}</div>
      </div>
    </div>
  );
}

/* ---------- helpers ---------- */

function fmtDate(iso: string): string {
  if (!iso) return "—";
  try {
    return new Date(iso).toLocaleString();
  } catch {
    return iso;
  }
}

function renderCell(v: unknown, t: FieldType): string {
  if (v == null) return "—";
  if (t === "bool") return v ? "✓" : "✗";
  if (t === "json") return typeof v === "string" ? v : JSON.stringify(v);
  return String(v);
}

function RuleBadge(props: { op: string; rule: string }) {
  const rule = (props.rule || "authed") as "public" | "authed" | "admin";
  const tone =
    rule === "public" ? "bg-copper-50 text-copper-700 border-copper-100" :
    rule === "admin"  ? "bg-canvas-100 text-ink-700 border-canvas-300" :
                        "bg-steel-100 text-steel-700 border-steel-100";
  return (
    <span className={`inline-flex items-center gap-1 rounded-full border px-2 py-0.5 font-mono ${tone}`}>
      <span className="opacity-60">{props.op}:</span>{rule}
    </span>
  );
}

function typeChipCls(t: FieldType): string {
  switch (t) {
    case "text":
      return "";
    case "number":
      return "chip-steel";
    case "bool":
      return "chip-steel";
    case "json":
      return "chip-copper";
    case "date":
      return "chip-steel";
  }
}

function coerceForType(v: unknown, t: FieldType): unknown {
  if (v === null || v === undefined) return v;
  switch (t) {
    case "number": {
      if (v === "") return null;
      const n = Number(v);
      return Number.isNaN(n) ? v : n;
    }
    case "bool":
      return Boolean(v);
    case "json": {
      if (typeof v !== "string") return v;
      try {
        return JSON.parse(v);
      } catch {
        return v;
      }
    }
    default:
      return v;
  }
}
