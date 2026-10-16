import { create } from "zustand";
import { buildApiUrl } from "@/lib/http";
import { useAuthStore } from "@/store/auth-store";
import { toast } from "@/components/common/toast-store";

type MessageState = {
  publicReady: boolean;
  privateReady: boolean;
  publicVersion: number;
  privateVersion: number;
  notificationVersion: number;
  status: "connecting" | "connected" | "reconnecting";
  publicStatus: "connecting" | "connected" | "reconnecting";
  dismissedAnnouncements: Set<number>;
};
export const useMessageEvents = create<MessageState>(() => ({ publicReady: false, privateReady: false, publicVersion: 0, privateVersion: 0, notificationVersion: 0, status: "connecting", publicStatus: "connecting", dismissedAnnouncements: new Set() }));

function pause(ms: number, signal: AbortSignal) {
  return new Promise<void>((resolve) => {
    const finish = () => { window.clearTimeout(timer); signal.removeEventListener("abort", finish); resolve(); };
    const timer = window.setTimeout(finish, ms);
    signal.addEventListener("abort", finish, { once: true });
    if (signal.aborted) finish();
  });
}

export function connectMessages(token: string | null) {
  const controller = new AbortController();
  const signal = controller.signal;
  const privateStream = Boolean(token);
  const path = privateStream ? "/api/system/notification/events" : "/api/public/announcement/events";
  const seen = new Set<number>();
  if (privateStream) useMessageEvents.setState({ privateReady: false, status: "connecting" });
  else useMessageEvents.setState({ publicReady: false, publicStatus: "connecting" });

  const receive = (value: { type: string; id?: number }) => {
    if (signal.aborted) return;
    if (value.type === "heartbeat") return;
    useMessageEvents.setState((state) => {
      if (value.type === "ready") return privateStream
        ? { privateReady: true, status: "connected", privateVersion: state.privateVersion + 1, notificationVersion: state.notificationVersion + 1 }
        : { publicReady: true, publicStatus: "connected", publicVersion: state.publicVersion + 1 };
      if (value.type === "announcement") return privateStream ? { privateVersion: state.privateVersion + 1 } : { publicVersion: state.publicVersion + 1 };
      if (value.type === "notification" || value.type === "read") return { notificationVersion: state.notificationVersion + 1 };
      return state;
    });
    if (privateStream && value.type === "notification" && value.id && !seen.has(value.id)) {
      seen.add(value.id);
      if (seen.size > 200) seen.delete(seen.values().next().value!);
      if (document.visibilityState === "visible" && document.hasFocus()) toast.info("收到新的站内通知");
    }
  };

  void (async () => {
    let attempt = 0;
    while (!signal.aborted) {
      try {
        const response = await fetch(buildApiUrl(path), { signal, headers: token ? { Authorization: token, Accept: "text/event-stream" } : { Accept: "text/event-stream" }, cache: "no-store" });
        if (signal.aborted) { await response.body?.cancel().catch(() => undefined); return; }
        if (response.status === 401 && privateStream) { useAuthStore.getState().clearAuth(); return; }
        if (!response.ok || !response.headers.get("Content-Type")?.startsWith("text/event-stream") || !response.body) throw new Error("消息连接暂不可用");
        const reader = response.body.getReader();
        const decoder = new TextDecoder();
        let buffer = "";
        try {
          while (!signal.aborted) {
            const { value, done } = await reader.read();
            if (done) break;
            buffer += decoder.decode(value, { stream: true }).replace(/\r\n/g, "\n");
            if (buffer.length > 65536) throw new Error("消息连接数据异常");
            let end: number;
            while ((end = buffer.indexOf("\n\n")) !== -1) {
              const frame = buffer.slice(0, end); buffer = buffer.slice(end + 2);
              const data = frame.split("\n").filter((line) => line.startsWith("data:")).map((line) => line.slice(5).trimStart()).join("\n");
              if (data) { const event = JSON.parse(data) as { type: string; id?: number }; receive(event); if (event.type === "ready") attempt = 0; }
            }
          }
        } finally { await reader.cancel().catch(() => undefined); reader.releaseLock(); }
      } catch { /* Reconnect without querying message endpoints on a timer. */ }
      if (signal.aborted) break;
      if (privateStream) useMessageEvents.setState({ status: "reconnecting" });
      else useMessageEvents.setState({ publicStatus: "reconnecting" });
      await pause(Math.min(1000 * 2 ** attempt++, 15000), signal);
    }
  })();
  return () => {
    controller.abort();
    if (privateStream) useMessageEvents.setState({ privateReady: false });
    else useMessageEvents.setState({ publicReady: false });
  };
}

export function safeMessagePath(path?: string) {
  if (!path || !path.startsWith("/") || path.startsWith("//") || /[\\\r\n]/.test(path)) return null;
  try {
    const decoded = decodeURIComponent(path);
    if (decoded.startsWith("//") || /[\\\r\n]/.test(decoded)) return null;
    const target = new URL(path, window.location.origin);
    return target.origin === window.location.origin ? `${target.pathname}${target.search}${target.hash}` : null;
  } catch { return null; }
}
