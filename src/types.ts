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
  rpcOriginCheckMode: 'disabled' | 'same_origin' | 'whitelist'
  rpcOriginWhitelist: string[]
  trackerSubscriptionEnabled: boolean
  trackerSubscriptionSource: string
  mcpEnabled: boolean
  refreshIntervalMs: number
  defaultDownloadDir: string
  theme: 'ariamx'
  colorMode: 'system' | 'light' | 'dark'
  skinEnabled: boolean
  skinName: string
  skinApiTemplate: string
}

export type PublicPanelStyle = {
  theme: 'ariamx'
  colorMode: 'system' | 'light' | 'dark'
  skinEnabled: boolean
  skinName: string
  skinApiTemplate: string
}

export type TrackerSubscriptionState = {
  enabled: boolean
  selectedSource: string
  currentTrackerCount: number
  message?: string
}

export type AppAbout = {
  panelVersion: string
  aria2Version: string
  rpcPath: string
  httpRpcUrl: string
  wsRpcUrl: string
  mcpHttpUrl: string
  mcpEnabled: boolean
  panelRpcSecret: string
}

export type ManagedOptionsSaveResult = {
  restarted: boolean
  message: string
}

export type ScriptHookItem = {
  key: string
  title: string
  content: string
}

export type ScriptHookState = {
  hooks: ScriptHookItem[]
  message?: string
}

export type PeerBanRecord = {
  ip: string
  reason?: string
  createdAt?: string
  expiresAt?: string
}

export type SuspiciousPeer = {
  gid: string
  taskName: string
  ip: string
  port: string
  downloadSpeed: string
  uploadSpeed: string
  seeder: boolean
  blocked: boolean
  score: number
  reason: string
}

export type PeerGuardSnapshot = {
  firewallMode: string
  firewallReady: boolean
  firewallOperable: boolean
  actionBlockedReason?: string
  lastError?: string
  lastAppliedAt?: string
  autoBanEnabled: boolean
  autoBanMinScore: number
  blockedPeers: PeerBanRecord[]
  suspiciousPeers: SuspiciousPeer[]
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
