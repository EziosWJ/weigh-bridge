import { http } from "@/lib/http";

export type Announcement = {
  id: number;
  title: string;
  content: string;
  scope: "PUBLIC" | "INTERNAL";
  status: "DRAFT" | "PUBLISHED" | "WITHDRAWN";
  publishTime?: string | null;
  expiresAt?: string | null;
  createTime: string;
};
export type AnnouncementInput = Pick<Announcement, "title" | "content" | "scope" | "expiresAt">;
export type AnnouncementPage = { records: Announcement[]; total: number; page: number; pageSize: number };
const base = (scope: Announcement["scope"]) => scope === "PUBLIC" ? "/api/public/announcement" : "/api/system/announcement";
export const getAnnouncements = (scope: Announcement["scope"], page = 1, pageSize = 10, signal?: AbortSignal) => http.get<AnnouncementPage>(`${base(scope)}/page`, { query: { page, pageSize }, signal });
export const getAnnouncement = (item: Pick<Announcement, "id" | "scope">, signal?: AbortSignal) => http.get<Announcement>(`${base(item.scope)}/${item.id}`, { signal });
export const getAdminAnnouncements = (page: number) => http.get<AnnouncementPage>("/api/system/announcement-admin/page", { query: { page, pageSize: 10 } });
export const saveAnnouncement = (value: AnnouncementInput, id?: number) => id ? http.put<Announcement>(`/api/system/announcement/${id}`, value) : http.post<Announcement>("/api/system/announcement", value);
export const publishAnnouncement = (id: number) => http.put<void>(`/api/system/announcement/${id}/publish`);
export const withdrawAnnouncement = (id: number) => http.put<void>(`/api/system/announcement/${id}/withdraw`);
