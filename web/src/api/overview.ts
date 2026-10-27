import { http } from '@/lib/http';
import type { WireTask, WireTicket } from './weighing-workflow';

export interface BusinessOverview {
  date: string;
  checkedAt: string;
  validTickets: number;
  inbound: { taskCount: number; net: string };
  outbound: { taskCount: number; net: string };
  awaitingCount: number;
  overnightCount: number;
  awaitingTasks: (WireTask & { firstConfirmedAt: string })[];
  recentTickets: WireTicket[];
}

export interface OverviewDevice {
  online: boolean;
  deviceOk: boolean;
  stable: boolean;
  error?: string;
  receivedAt: string;
  captureRequired: boolean;
  captureBlockedReason?: string;
}

export const overviewApi = {
  business: () => http.get<BusinessOverview>('/api/weighing/overview'),
  device: (code: string) => http.get<OverviewDevice>(`/api/weighing/scales/${code}/device`),
};
