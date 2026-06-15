export type ApiResponse<T> = {
  ok: boolean
  data?: T
  error?: {
    code: string
    message: string
  }
}

export type CurrentUser = {
  username: string
  sessionExpiresAt: string
}

export type AppConfig = {
  aria2RpcUrl: string
  hasAria2Secret: boolean
  aria2Managed: boolean
  managedRpcPort: number
  refreshIntervalMs: number
  defaultDownloadDir: string
  theme: 'classic' | 'design'
}

export type AppAbout = {
  panelVersion: string
  aria2Version: string
  rpcPath: string
}

export type ManagedOptionsSaveResult = {
  restarted: boolean
  message: string
}

export type Aria2File = {
  index: string
  path: string
  length: string
  completedLength: string
  selected: string
  uris?: Aria2Uri[]
}

export type Aria2Uri = {
  uri: string
  status: string
}

export type Aria2Peer = {
  peerId?: string
  ip: string
  port: string
  bitfield?: string
  amChoking?: string
  peerChoking?: string
  downloadSpeed: string
  uploadSpeed: string
  seeder?: string
}

export type Aria2Server = {
  index: string
  servers: Array<{
    uri: string
    currentUri?: string
    downloadSpeed: string
  }>
}

export type Aria2OptionMap = Record<string, string>

export type Aria2Bittorrent = {
  announceList?: string[][]
  comment?: string
  creationDate?: number
  mode?: string
  info?: {
    name?: string
    verifiedLength?: string
    verifyIntegrityPending?: string
  }
}

export type Aria2Task = {
  gid: string
  status: string
  totalLength: string
  completedLength: string
  downloadSpeed: string
  uploadSpeed: string
  uploadLength?: string
  numSeeders?: string
  connections?: string
  dir?: string
  files?: Aria2File[]
  bittorrent?: Aria2Bittorrent
  followedBy?: string[]
  following?: string
  belongsTo?: string
  numPieces?: string
  pieceLength?: string
  seeder?: string
  errorCode?: string
  errorMessage?: string
}

export type GlobalStat = {
  downloadSpeed: string
  uploadSpeed: string
  numActive: string
  numWaiting: string
  numStopped: string
}

export type TaskBucket = 'active' | 'waiting' | 'stopped'
