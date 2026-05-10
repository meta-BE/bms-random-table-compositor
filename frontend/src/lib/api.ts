// Wails Bind のラッパ。生成型の細かい変動を吸収するために薄く包む。
import {
  GetServerConfig,
  SetServerPort,
  SetSongdataDBPath,
  PickSongdataDB,
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
  GetPublishedTable,
  CreatePublishedTable,
  CreatePublishedTableFromSource,
  UpdatePublishedTable,
  ApplyBulkPickConfig,
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
  GetSongdataAttachStatus,
  ReattachSongdata,
} from '../../wailsjs/go/handler/SongdataHandler';
import { Snapshot as DashboardSnapshot } from '../../wailsjs/go/handler/DashboardHandler';
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

export interface PublishedTableLevelMappingDTO {
  id: string;
  sourceTableId: string;
  sourceLevel: string;
  sortOrder: number;
}

export interface PublishedTableLevelDTO {
  id: string;
  name: string;
  sortOrder: number;
  perMappingPick: number;
  totalPick: number;
  mappings: PublishedTableLevelMappingDTO[];
}

export interface PublishedTableDTO {
  id: string;
  slug: string;
  displayName: string;
  symbol: string;
  ownedOnly: boolean;
  refreshMode: RefreshMode;
  sortOrder: number;
  levels: PublishedTableLevelDTO[];
}

export interface PublishedTableLevelMappingInputDTO {
  sourceTableId: string;
  sourceLevel: string;
}

export interface PublishedTableLevelInputDTO {
  name: string;
  perMappingPick: number;
  totalPick: number;
  mappings: PublishedTableLevelMappingInputDTO[];
}

export interface CreatePublishedTableRequest {
  slug: string;
  displayName: string;
  symbol: string;
  ownedOnly: boolean;
  refreshMode: RefreshMode;
  levels: PublishedTableLevelInputDTO[];
}

export interface UpdatePublishedTableRequest extends CreatePublishedTableRequest {
  id: string;
  sortOrder: number;
}

export interface CreateFromSourceRequest {
  sourceTableId: string;
  slug: string;
  displayName: string;
  symbol: string;
}

export interface ApplyBulkPickConfigRequest {
  id: string;
  perMappingPick: number;
  totalPick: number;
}

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

export type SongdataAttachStatusDTO = {
  attached: boolean;
  path: string;
  songCount: number;
  attachedAt: string;
  lastError: string;
};

export type RequestLogDTO = {
  at: string;
  method: string;
  path: string;
  slug: string;
  statusCode: number;
  durationMs: number;
};

export type FetchLogDTO = {
  at: string;
  sourceId: string;
  displayName: string;
  status: 'never' | 'ok' | 'error';
  error: string;
};

export type PickSnapshotDTO = {
  publishedId: string;
  generatedAt: string;
  levelOrder: string[];
  levelCounts: Record<string, number>;
  totalCount: number;
};

export type DashboardSnapshotDTO = {
  requests: RequestLogDTO[];
  fetches: FetchLogDTO[];
  picks: PickSnapshotDTO[];
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
  pickSongdataDB(): Promise<string> {
    return PickSongdataDB() as Promise<string>;
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
  getPublishedTable(id: string): Promise<PublishedTableDTO> {
    return GetPublishedTable(id) as Promise<PublishedTableDTO>;
  },
  createPublishedTable(req: CreatePublishedTableRequest): Promise<string> {
    // Wails 生成型は convertValues メソッドを含むクラス。POJO を渡せば JSON ラウンドトリップで動作する
    return CreatePublishedTable(req as any) as Promise<string>;
  },
  createPublishedTableFromSource(req: CreateFromSourceRequest): Promise<string> {
    return CreatePublishedTableFromSource(req as any) as Promise<string>;
  },
  updatePublishedTable(req: UpdatePublishedTableRequest): Promise<void> {
    return UpdatePublishedTable(req as any);
  },
  applyBulkPickConfig(req: ApplyBulkPickConfigRequest): Promise<void> {
    return ApplyBulkPickConfig(req as any);
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
  // ---- songdata.db アタッチ ----
  getSongdataAttachStatus(): Promise<SongdataAttachStatusDTO> {
    return GetSongdataAttachStatus() as Promise<SongdataAttachStatusDTO>;
  },
  reattachSongdata(): Promise<void> {
    return ReattachSongdata();
  },
  // ---- イベント ----
  onSourceTableRefreshAllDone(cb: () => void): () => void {
    EventsOn('source_table:refresh_all_done', cb);
    return () => EventsOff('source_table:refresh_all_done');
  },
  // ---- ダッシュボード ----
  getDashboardSnapshot(): Promise<DashboardSnapshotDTO> {
    return DashboardSnapshot() as Promise<DashboardSnapshotDTO>;
  },
  onDashboardRequestLogged(cb: (e: RequestLogDTO) => void): () => void {
    EventsOn('dashboard:request_logged', cb);
    return () => EventsOff('dashboard:request_logged');
  },
  onDashboardFetchLogged(cb: (e: FetchLogDTO) => void): () => void {
    EventsOn('dashboard:fetch_logged', cb);
    return () => EventsOff('dashboard:fetch_logged');
  },
  onDashboardPickChanged(cb: (publishedID: string) => void): () => void {
    EventsOn('dashboard:pick_changed', cb);
    return () => EventsOff('dashboard:pick_changed');
  },
};
