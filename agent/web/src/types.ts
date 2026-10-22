export interface Config {
  siteId: number; siteCode: string; siteName: string; scaleCode: string; scaleName: string;
  serverUrl: string; agentId: string; scaleId: number; agentKey: string; queue: string;
  printWidthMm: number; printScale: number; offsetXmm: number; offsetYmm: number; formLengthMm: number;
  serialEnabled: boolean; serialPort: string; serialBaud: number; serialProtocol: string;
  serialWeightUnit: string; serialStaleMs: number;
}
export interface ConfigData {
  config: Config; active: Config; hasPrevious: boolean; hasAgentKey: boolean;
  hasActiveAgentKey: boolean; keyChanged: boolean; saved: boolean;
  environmentKnown: boolean; environmentError: string; runtimeState: string;
}
export interface Job { id: string; queue: string; user?: string; title?: string; state: string }
export interface JobsData {
  jobs: Job[] | null; previewToken: string; unknown: boolean; queuesOutput: string;
}
export interface Scale {
  known: boolean; enabled: boolean; state: string; stable: boolean; instrumentStable: boolean;
  weight: string; unit: string; lastValidAt: string; fault: string; port: string; baud: number;
  protocol: string; sourceWeight: string; sourceTare: string; weightType: string;
  checksum: string; checksumFailures: number; framesValid: number; framesInvalid: number;
  discardedBytes: number; unitFlag: string; negative: boolean; anomaly: boolean; tareIssue?: string;
}
export interface Status {
  checkedAt: string; maintenance: string; agent: string; agentKnown: boolean;
  cups: string; cupsKnown: boolean; cloud: string; queue: string; queueState: string;
  queues: string[]; queueKnown: boolean; queueDetails: { name: string; state: string }[];
  jobsKnown: boolean; jobs: Job[]; lastError: string; scale: Scale;
}
export interface ActionResult {
  needsDisposition?: boolean; previewToken: string; jobs?: Job[]; unknown?: boolean; output?: string;
}
export interface Audit {
  at: string; source: string; session: string; action: string; target?: string;
  result: string; before?: unknown; after?: unknown;
}
export interface AuditData { records: Audit[]; total: number; offset: number; limit: number }
