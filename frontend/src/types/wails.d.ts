export type DBStats = {
  dataDir: string;
  walPath: string;
  walBytes: number;
  keys: number;
  liveKeys: number;
  tombstones: number;
  memtableBytes: number;
  ssTableCount: number;
};

export type DBServiceApi = {
  Get(key: string): Promise<string>;
  Set(key: string, value: string): Promise<void>;
  Delete(key: string): Promise<void>;
  GetStats(): Promise<DBStats>;
  ListKeys(opts: { prefix: string; limit: number }): Promise<string[]>;
  Compact(): Promise<void>;
  Scan(opts: { prefix: string; after: string; limit: number }): Promise<{ keys: string[]; nextAfter: string }>;
};

declare global {
  interface Window {
    go?: {
      bridge?: {
        DBService?: DBServiceApi;
      };
    };
  }
}
