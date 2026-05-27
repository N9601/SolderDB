import { useCallback, useEffect, useRef, useState } from "react";
import { GetAPIAddr } from "../wailsjs/go/bridge/DBService";
import { apiFetch, apiJSON, withAuthQuery } from "../lib/apiFetch";

type FileMeta = {
  id: string;
  name: string;
  size: number;
  mimeType: string;
  sha256: string;
  created: string;
};

type Props = {
  onStatus: (s: string) => void;
};

function formatBytes(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes <= 0) return "0 B";
  const units = ["B", "KB", "MB", "GB"];
  let v = bytes;
  let i = 0;
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024;
    i++;
  }
  return `${v.toFixed(i === 0 ? 0 : 1)} ${units[i]}`;
}

export default function FilesView({ onStatus }: Props) {
  const [apiAddr, setApiAddr] = useState("");
  const [files, setFiles] = useState<FileMeta[]>([]);
  const [dragOver, setDragOver] = useState(false);
  const [uploading, setUploading] = useState(false);
  const inputRef = useRef<HTMLInputElement | null>(null);

  useEffect(() => {
    void (async () => {
      try {
        setApiAddr(await GetAPIAddr());
      } catch {
        // ignore
      }
    })();
  }, []);

  const refresh = useCallback(async () => {
    if (!apiAddr) return;
    try {
      const data = await apiJSON<{ files?: FileMeta[] }>(`${apiAddr}/api/files?limit=200`);
      setFiles(data.files ?? []);
    } catch (e) {
      onStatus(`Files list error: ${String(e)}`);
    }
  }, [apiAddr, onStatus]);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  // SSE: refresh on file collection changes.
  useEffect(() => {
    if (!apiAddr) return;
    const es = new EventSource(withAuthQuery(`${apiAddr}/api/realtime?topic=coll:_files`));
    const tick = () => void refresh();
    es.addEventListener("create", tick);
    es.addEventListener("delete", tick);
    return () => es.close();
  }, [apiAddr, refresh]);

  async function uploadFile(file: File) {
    if (!apiAddr) {
      onStatus("API not running");
      return;
    }
    setUploading(true);
    try {
      const form = new FormData();
      form.append("file", file);
      const res = await apiFetch(`${apiAddr}/api/files`, { method: "POST", body: form });
      if (!res.ok) {
        const body = await res.json().catch(() => ({}) as { error?: string });
        throw new Error(body.error ?? `HTTP ${res.status}`);
      }
      onStatus(`Uploaded ${file.name}`);
      await refresh();
    } catch (e) {
      onStatus(`Upload error: ${String(e)}`);
    } finally {
      setUploading(false);
    }
  }

  async function uploadList(list: FileList | null) {
    if (!list) return;
    for (let i = 0; i < list.length; i++) {
      const f = list.item(i);
      if (f) await uploadFile(f);
    }
  }

  async function onDelete(id: string, name: string) {
    if (!apiAddr) return;
    if (!window.confirm(`Delete "${name}"?`)) return;
    try {
      const res = await apiFetch(`${apiAddr}/api/files/${id}`, { method: "DELETE" });
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      onStatus(`Deleted ${name}`);
      await refresh();
    } catch (e) {
      onStatus(`Delete error: ${String(e)}`);
    }
  }

  function isImage(m: string) {
    return m.startsWith("image/");
  }

  return (
    <div className="animate-slideUp space-y-5">
      {/* Drop zone */}
      <div
        className={`card flex flex-col items-center justify-center border-2 border-dashed py-10 transition-colors ${
          dragOver ? "border-copper-500 bg-copper-50" : "border-canvas-300"
        }`}
        onDragOver={(e) => {
          e.preventDefault();
          setDragOver(true);
        }}
        onDragLeave={() => setDragOver(false)}
        onDrop={(e) => {
          e.preventDefault();
          setDragOver(false);
          void uploadList(e.dataTransfer.files);
        }}
      >
        <svg width="32" height="32" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" className="text-ink-400">
          <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4" />
          <polyline points="17 8 12 3 7 8" />
          <line x1="12" y1="3" x2="12" y2="15" />
        </svg>
        <div className="mt-2 text-[14px] font-semibold text-ink-900">
          {uploading ? "Uploading…" : "Drop files here or click to upload"}
        </div>
        <div className="mt-1 text-[12px] text-ink-400">Max 100 MB · stored on disk, hashed with SHA-256</div>
        <button className="btn btn-primary mt-4" onClick={() => inputRef.current?.click()} disabled={uploading}>
          Choose file
        </button>
        <input
          ref={inputRef}
          type="file"
          multiple
          className="hidden"
          onChange={(e) => void uploadList(e.target.files)}
        />
      </div>

      {/* List */}
      <div className="card overflow-hidden">
        <div className="flex items-center justify-between border-b border-canvas-200 px-4 py-3">
          <div className="section-title">{files.length} files</div>
          <button className="btn btn-ghost" onClick={() => void refresh()}>
            Refresh
          </button>
        </div>
        {files.length === 0 ? (
          <div className="px-4 py-10 text-center text-[12px] text-ink-400">No files yet, drop one above.</div>
        ) : (
          <div className="grid grid-cols-2 gap-3 p-4 md:grid-cols-3 lg:grid-cols-4">
            {files.map((f) => (
              <div key={f.id} className="group flex flex-col overflow-hidden rounded-lg border border-canvas-200 bg-white transition-shadow hover:shadow-cardHover">
                <div className="flex h-32 items-center justify-center overflow-hidden bg-canvas-100">
                  {isImage(f.mimeType) && apiAddr ? (
                    // eslint-disable-next-line jsx-a11y/img-redundant-alt
                    <img
                      src={withAuthQuery(`${apiAddr}/api/files/${f.id}`)}
                      alt={f.name}
                      className="h-full w-full object-cover"
                      loading="lazy"
                    />
                  ) : (
                    <div className="text-[11px] font-semibold uppercase tracking-wider text-ink-400">
                      {f.mimeType.split("/")[1]?.slice(0, 8) ?? "file"}
                    </div>
                  )}
                </div>
                <div className="flex-1 p-3">
                  <div className="truncate text-[12.5px] font-medium text-ink-900" title={f.name}>
                    {f.name}
                  </div>
                  <div className="mt-0.5 text-[11px] text-ink-400">
                    {formatBytes(f.size)} · {f.mimeType.split(";")[0]}
                  </div>
                  <div className="mt-0.5 truncate font-mono text-[10px] text-ink-300" title={f.sha256}>
                    {f.sha256.slice(0, 14)}…
                  </div>
                  <div className="mt-2 flex gap-1.5">
                    <a
                      className="btn btn-ghost flex-1 justify-center"
                      href={withAuthQuery(`${apiAddr}/api/files/${f.id}`)}
                      target="_blank"
                      rel="noreferrer"
                    >
                      Open
                    </a>
                    <button className="btn btn-ghost btn-danger" onClick={() => void onDelete(f.id, f.name)}>
                      ✕
                    </button>
                  </div>
                </div>
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}
