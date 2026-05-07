// Wails Bind のラッパ。生成型の細かい変動を吸収するために薄く包む。
import {
  GetServerConfig,
  SetServerPort,
  SetSongdataDBPath,
} from '../../wailsjs/go/handler/ConfigHandler';

export type ServerConfig = {
  port: number;
  songdataDbPath: string;
};

export const api = {
  getServerConfig(): Promise<ServerConfig> {
    return GetServerConfig() as Promise<ServerConfig>;
  },
  setServerPort(port: number): Promise<void> {
    return SetServerPort(port);
  },
  setSongdataDBPath(path: string): Promise<void> {
    return SetSongdataDBPath(path);
  },
};
