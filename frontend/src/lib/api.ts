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
import {
  ListPublishedTables,
  CreatePublishedTable,
  UpdatePublishedTable,
  DeletePublishedTable,
  ValidateSlug,
  SuggestSlugFromSource,
  OpenPublishedTableURL,
} from '../../wailsjs/go/handler/PublishedTableHandler';
import { ManualRefreshPick } from '../../wailsjs/go/handler/PickHandler';
import {
  GetServerStatus,
  StartServer,
  StopServer,
  RestartServer,
} from '../../wailsjs/go/handler/ServerStatusHandler';
import {
  GetOwnedCacheStatus,
  ReloadOwnedCache,
} from '../../wailsjs/go/handler/OwnedChartHandler';
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

export type AddSourceTableRequest = { url: string };

export type RefreshMode = 'per_request' | 'daily' | 'manual';

export type PublishedTableDTO = {
  id: string;
  slug: string;
  displayName: string;
  symbol: string;
  sourceTableId: string;
  ownedOnly: boolean;
  pickPerLevel: number;
  refreshMode: RefreshMode;
  sortOrder: number;
};

export type CreatePublishedTableRequest = {
  slug: string;
  displayName: string;
  symbol: string;
  sourceTableId: string;
  ownedOnly: boolean;
  pickPerLevel: number;
  refreshMode: RefreshMode;
};

export type UpdatePublishedTableRequest = CreatePublishedTableRequest & {
  id: string;
  sortOrder: number;
};

export type SlugValidation =
  | { ok: true; reason?: undefined }
  | { ok: false; reason: 'invalid_format' | 'reserved' | 'duplicate' | string };

export type ServerState = 'stopped' | 'running' | 'error';

export type ServerStatusDTO = {
  state: ServerState;
  port: number;
  startedAt: string;
  lastError: string;
};

export type OwnedCacheStatusDTO = {
  loaded: boolean;
  count: number;
  loadedAt: string;
  loadedPath: string;
  lastError: string;
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
  // ---- 公開表 ----
  listPublishedTables(): Promise<PublishedTableDTO[]> {
    return ListPublishedTables() as Promise<PublishedTableDTO[]>;
  },
  createPublishedTable(req: CreatePublishedTableRequest): Promise<string> {
    return CreatePublishedTable(req) as Promise<string>;
  },
  updatePublishedTable(req: UpdatePublishedTableRequest): Promise<void> {
    return UpdatePublishedTable(req);
  },
  deletePublishedTable(id: string): Promise<void> {
    return DeletePublishedTable(id);
  },
  validateSlug(slug: string, excludeId: string): Promise<SlugValidation> {
    return ValidateSlug(slug, excludeId) as Promise<SlugValidation>;
  },
  suggestSlugFromSource(sourceId: string): Promise<string> {
    return SuggestSlugFromSource(sourceId) as Promise<string>;
  },
  openPublishedTableURL(slug: string, port: number): Promise<void> {
    return OpenPublishedTableURL(slug, port);
  },
  manualRefreshPick(publishedId: string): Promise<void> {
    return ManualRefreshPick(publishedId);
  },
  // ---- サーバ ----
  getServerStatus(): Promise<ServerStatusDTO> {
    return GetServerStatus() as Promise<ServerStatusDTO>;
  },
  startServer(): Promise<void> {
    return StartServer();
  },
  stopServer(): Promise<void> {
    return StopServer();
  },
  restartServer(): Promise<void> {
    return RestartServer();
  },
  onServerStatusChanged(cb: (s: ServerStatusDTO) => void): () => void {
    EventsOn('server_status:changed', cb);
    return () => EventsOff('server_status:changed');
  },
  // ---- 所持キャッシュ ----
  getOwnedCacheStatus(): Promise<OwnedCacheStatusDTO> {
    return GetOwnedCacheStatus() as Promise<OwnedCacheStatusDTO>;
  },
  reloadOwnedCache(): Promise<void> {
    return ReloadOwnedCache();
  },
  // ---- イベント ----
  onSourceTableRefreshAllDone(cb: () => void): () => void {
    EventsOn('source_table:refresh_all_done', cb);
    return () => EventsOff('source_table:refresh_all_done');
  },
};
