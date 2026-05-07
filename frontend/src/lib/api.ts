// Wails Bind のラッパ。生成型の細かい変動を吸収するために薄く包む。
import {
  GetServerConfig,
  SetServerPort,
  SetSongdataDBPath,
} from '../../wailsjs/go/handler/ConfigHandler';
import {
  ListSourceTables,
  AddSourceTable,
  RefreshSourceTable,
  RefreshAllSourceTables,
  DeleteSourceTable,
  UpdateSourceTableDisplayName,
} from '../../wailsjs/go/handler/SourceTableHandler';
import { EventsOn, EventsOff } from '../../wailsjs/runtime/runtime';

export type ServerConfig = {
  port: number;
  songdataDbPath: string;
};

export type SourceTableDTO = {
  id: string;
  inputUrl: string;
  inputKind: 'html' | 'header_json';
  displayName: string;
  name: string;
  symbol: string;
  levelOrder: string[];
  dataUrl: string;
  lastFetchedAt: string;
  lastFetchStatus: 'never' | 'ok' | 'error';
  lastFetchError: string;
};

// AddSourceTable の入力は URL のみ。InputKind はバックエンドで URL 拡張子から
// 自動判別し、DisplayName は取得後に Name でフォールバック表示する責務を
// フロントエンドが持つ。
export type AddSourceTableRequest = {
  url: string;
};

export const api = {
  // ---- 設定 ----
  getServerConfig(): Promise<ServerConfig> {
    return GetServerConfig() as Promise<ServerConfig>;
  },
  setServerPort(port: number): Promise<void> {
    return SetServerPort(port);
  },
  setSongdataDBPath(path: string): Promise<void> {
    return SetSongdataDBPath(path);
  },
  // ---- ソース表 ----
  listSourceTables(): Promise<SourceTableDTO[]> {
    return ListSourceTables() as Promise<SourceTableDTO[]>;
  },
  addSourceTable(req: AddSourceTableRequest): Promise<string> {
    return AddSourceTable(req) as Promise<string>;
  },
  refreshSourceTable(id: string): Promise<void> {
    return RefreshSourceTable(id);
  },
  refreshAllSourceTables(): Promise<void> {
    return RefreshAllSourceTables();
  },
  deleteSourceTable(id: string): Promise<void> {
    return DeleteSourceTable(id);
  },
  updateSourceTableDisplayName(id: string, displayName: string): Promise<void> {
    return UpdateSourceTableDisplayName(id, displayName);
  },
  // ---- イベント ----
  onSourceTableRefreshAllDone(cb: () => void): () => void {
    EventsOn('source_table:refresh_all_done', cb);
    return () => EventsOff('source_table:refresh_all_done');
  },
};
