export type DBStats = {
  dataDir: string;
  walPath: string;
  walBytes: number;
  keys: number;
  liveKeys: number;
  tombstones: number;
  memtableBytes: number;
};

export type DBServiceApi = {
  Get(key: string): Promise<string>;
  Set(key: string, value: string): Promise<void>;
  Delete(key: string): Promise<void>;
  GetStats(): Promise<DBStats>;
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

